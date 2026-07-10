//go:build windows

package agent

import (
	"encoding/binary"
	"log"
	"strings"
	"sync"
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
	a.audio = &audioRuntime{stop: make(chan struct{})}
	return a.audio
}

func (a *Agent) stopAudioLocked() {
	if a.audio == nil {
		return
	}
	a.audio.stopOnce.Do(func() { close(a.audio.stop) })
	a.audio = nil
}

func (a *Agent) startHostMic(track *webrtc.TrackLocalStaticSample, gen uint64) {
	a.mu.Lock()
	rt := a.ensureAudioRuntimeLocked()
	stop := rt.stop
	a.mu.Unlock()

	go func() {
		ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
		if err != nil {
			log.Printf("agent: audio mic context: %v (voice send disabled)", err)
			return
		}
		defer func() {
			_ = ctx.Uninit()
			ctx.Free()
		}()

		var (
			mu     sync.Mutex
			pending []int16
		)

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
			mu.Lock()
			pending = append(pending, samples...)
			mu.Unlock()
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

		ticker := time.NewTicker(audioFrameMs * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-a.closed:
				return
			case <-ticker.C:
				a.mu.Lock()
				alive := a.sessGen == gen && a.pc != nil
				a.mu.Unlock()
				if !alive {
					return
				}
				mu.Lock()
				var frame []int16
				if len(pending) >= audioFramePCM {
					frame = append([]int16(nil), pending[:audioFramePCM]...)
					pending = pending[audioFramePCM:]
					if len(pending) > audioSampleRate { // drop backlog (~1s)
						pending = pending[len(pending)-audioFramePCM:]
					}
				} else {
					frame = make([]int16, audioFramePCM) // silence
				}
				mu.Unlock()
				payload := pcm16ToMulaw(frame)
				if err := track.WriteSample(media.Sample{
					Data:     payload,
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
	a.mu.Unlock()
	if rt == nil || len(samples) == 0 {
		return
	}
	rt.playMu.Lock()
	rt.playBuf = append(rt.playBuf, samples...)
	const maxBuf = audioSampleRate * 2 // ~2s
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
	stop := rt.stop
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
			a.mu.Unlock()
			var chunk []int16
			if rt != nil {
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

		<-stop
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
	// Max Opus frame ~120ms at 8kHz = 960 samples/channel
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
