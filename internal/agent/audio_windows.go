//go:build windows

package agent

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"strings"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/pion/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	audioSampleRate = 8000
	audioFrameMs    = 20
	audioFramePCM   = audioSampleRate * audioFrameMs / 1000 // 160
)

func (a *Agent) ensureAudioRuntimeLocked() *audioRuntime {
	if a.audio != nil {
		return a.audio
	}
	a.audio = &audioRuntime{ambientStop: make(chan struct{})}
	return a.audio
}

func (a *Agent) stopAmbientMicLocked() {
	if a.audio == nil {
		return
	}
	a.audio.ambientOnce.Do(func() {
		if a.audio.ambientStop != nil {
			close(a.audio.ambientStop)
		}
	})
	a.audio = nil
}

func (a *Agent) stopSessionAudioLocked() {
	if a.audio == nil {
		return
	}
	a.audio.playMu.Lock()
	a.audio.playBuf = nil
	a.audio.playStarted = false
	a.audio.playMu.Unlock()
}

func (a *Agent) stopAudioLocked() {
	a.stopSessionAudioLocked()
	a.stopAmbientMicLocked()
}

func (a *Agent) audioLevel() float64 {
	a.mu.Lock()
	rt := a.audio
	a.mu.Unlock()
	if rt == nil {
		return 0
	}
	rt.capMu.Lock()
	defer rt.capMu.Unlock()
	return rt.level
}

func pcmRMSLevel(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		v := float64(s) / 32768.0
		sum += v * v
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	// Soft knee so quiet speech still moves the meter.
	level := math.Min(1, rms*4)
	return level
}

// ensureAmbientMic starts always-on host mic capture for levels + session send.
func (a *Agent) ensureAmbientMic() {
	a.mu.Lock()
	rt := a.ensureAudioRuntimeLocked()
	if rt.micStarted {
		a.mu.Unlock()
		return
	}
	rt.micStarted = true
	stop := rt.ambientStop
	a.mu.Unlock()

	go a.runAmbientMic(stop)
	go a.pumpAudioLevels(stop)
}

func (a *Agent) runAmbientMic(stop <-chan struct{}) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		log.Printf("agent: audio mic context: %v (voice send disabled)", err)
		return
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = audioSampleRate
	cfg.PeriodSizeInFrames = audioFramePCM
	cfg.Alsa.NoMMap = 1

	onRecv := func(_, input []byte, frameCount uint32) {
		if len(input) < int(frameCount)*2 {
			return
		}
		n := int(frameCount)
		samples := make([]int16, n)
		for i := 0; i < n; i++ {
			samples[i] = int16(binary.LittleEndian.Uint16(input[i*2:]))
		}
		level := pcmRMSLevel(samples)
		a.mu.Lock()
		rt := a.audio
		a.mu.Unlock()
		if rt == nil {
			return
		}
		rt.capMu.Lock()
		rt.pending = append(rt.pending, samples...)
		if len(rt.pending) > audioSampleRate*2 {
			rt.pending = rt.pending[len(rt.pending)-audioSampleRate:]
		}
		// EMA for smoother VU.
		rt.level = rt.level*0.7 + level*0.3
		rt.capMu.Unlock()
	}

	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: onRecv})
	if err != nil {
		log.Printf("agent: audio mic device: %v (voice send disabled)", err)
		return
	}
	defer dev.Uninit()
	if err := dev.Start(); err != nil {
		log.Printf("agent: audio mic start: %v (voice send disabled)", err)
		return
	}
	log.Printf("agent: host mic capture started (%d Hz PCMU)", audioSampleRate)
	<-stop
}

func (a *Agent) pumpAudioLevels(stop <-chan struct{}) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-a.closed:
			return
		case <-ticker.C:
			level := a.audioLevel()
			payload, _ := json.Marshal(map[string]float64{"level": level})
			_ = a.send(signalingEnvelope{Type: "audio_level", Payload: payload})
		}
	}
}

// attachHostMicTrack pumps ambient capture into a WebRTC PCMU track for a session.
func (a *Agent) attachHostMicTrack(track *webrtc.TrackLocalStaticSample, gen uint64) {
	a.ensureAmbientMic()
	go func() {
		ticker := time.NewTicker(audioFrameMs * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-a.closed:
				return
			case <-ticker.C:
				a.mu.Lock()
				alive := a.sessGen == gen && a.pc != nil
				rt := a.audio
				a.mu.Unlock()
				if !alive {
					return
				}
				var frame []int16
				if rt != nil {
					rt.capMu.Lock()
					if len(rt.pending) >= audioFramePCM {
						frame = append([]int16(nil), rt.pending[:audioFramePCM]...)
						rt.pending = rt.pending[audioFramePCM:]
					} else {
						frame = make([]int16, audioFramePCM)
					}
					rt.capMu.Unlock()
				} else {
					frame = make([]int16, audioFramePCM)
				}
				if err := track.WriteSample(media.Sample{
					Data:     pcm16ToMulaw(frame),
					Duration: audioFrameMs * time.Millisecond,
				}); err != nil {
					return
				}
			}
		}
	}()
}

func (a *Agent) enqueuePlayback(samples []int16) {
	a.mu.Lock()
	rt := a.audio
	if rt == nil {
		rt = a.ensureAudioRuntimeLocked()
	}
	a.mu.Unlock()
	if len(samples) == 0 {
		return
	}
	rt.playMu.Lock()
	rt.playBuf = append(rt.playBuf, samples...)
	const maxBuf = audioSampleRate * 2
	if len(rt.playBuf) > maxBuf {
		rt.playBuf = rt.playBuf[len(rt.playBuf)-maxBuf:]
	}
	rt.playMu.Unlock()
}

func (a *Agent) ensurePlayback(gen uint64) {
	a.mu.Lock()
	rt := a.ensureAudioRuntimeLocked()
	if rt.playStarted {
		a.mu.Unlock()
		return
	}
	rt.playStarted = true
	stopAmbient := rt.ambientStop
	a.mu.Unlock()

	go func() {
		ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
		if err != nil {
			log.Printf("agent: audio play context: %v (voice recv disabled)", err)
			return
		}
		defer func() {
			_ = ctx.Uninit()
			ctx.Free()
		}()

		cfg := malgo.DefaultDeviceConfig(malgo.Playback)
		cfg.Playback.Format = malgo.FormatS16
		cfg.Playback.Channels = 1
		cfg.SampleRate = audioSampleRate
		cfg.PeriodSizeInFrames = audioFramePCM
		cfg.Alsa.NoMMap = 1

		onSend := func(output, _ []byte, frameCount uint32) {
			need := int(frameCount)
			a.mu.Lock()
			rt := a.audio
			alive := a.sessGen == gen
			a.mu.Unlock()
			var chunk []int16
			if rt != nil && alive {
				rt.playMu.Lock()
				if len(rt.playBuf) >= need {
					chunk = append([]int16(nil), rt.playBuf[:need]...)
					rt.playBuf = rt.playBuf[need:]
				} else if len(rt.playBuf) > 0 {
					chunk = append([]int16(nil), rt.playBuf...)
					rt.playBuf = rt.playBuf[:0]
				}
				rt.playMu.Unlock()
			}
			for i := 0; i < need; i++ {
				var s int16
				if i < len(chunk) {
					s = chunk[i]
				}
				binary.LittleEndian.PutUint16(output[i*2:], uint16(s))
			}
		}

		dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: onSend})
		if err != nil {
			log.Printf("agent: audio play device: %v (voice recv disabled)", err)
			return
		}
		defer dev.Uninit()
		if err := dev.Start(); err != nil {
			log.Printf("agent: audio play start: %v (voice recv disabled)", err)
			return
		}
		log.Printf("agent: speaker playback started (%d Hz)", audioSampleRate)

		for {
			select {
			case <-stopAmbient:
				return
			case <-a.closed:
				return
			case <-time.After(500 * time.Millisecond):
				a.mu.Lock()
				alive := a.sessGen == gen && a.audio != nil && a.audio.playStarted
				a.mu.Unlock()
				if !alive {
					return
				}
			}
		}
	}()
}

func (a *Agent) playRemoteAudio(track *webrtc.TrackRemote, gen uint64) {
	a.ensurePlayback(gen)

	mime := strings.ToLower(track.Codec().MimeType)
	log.Printf("agent: remote audio track %s", track.Codec().MimeType)

	switch {
	case strings.Contains(mime, "pcmu"):
		a.playRemotePCMU(track, gen)
	case strings.Contains(mime, "opus"):
		a.playRemoteOpus(track, gen)
	default:
		log.Printf("agent: unsupported remote audio codec %s", track.Codec().MimeType)
	}
}

func (a *Agent) playRemotePCMU(track *webrtc.TrackRemote, gen uint64) {
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		a.mu.Lock()
		alive := a.sessGen == gen
		a.mu.Unlock()
		if !alive {
			return
		}
		if pkt == nil || len(pkt.Payload) == 0 {
			continue
		}
		a.enqueuePlayback(mulawToPCM16(pkt.Payload))
	}
}

func (a *Agent) playRemoteOpus(track *webrtc.TrackRemote, gen uint64) {
	dec, err := opus.NewDecoderWithOutput(audioSampleRate, 1)
	if err != nil {
		log.Printf("agent: opus decoder: %v", err)
		return
	}
	pcm := make([]int16, 960)
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		a.mu.Lock()
		alive := a.sessGen == gen
		a.mu.Unlock()
		if !alive {
			return
		}
		if pkt == nil || len(pkt.Payload) == 0 {
			continue
		}
		n, err := dec.DecodeToInt16(pkt.Payload, pcm)
		if err != nil || n <= 0 {
			continue
		}
		a.enqueuePlayback(append([]int16(nil), pcm[:n]...))
	}
}
