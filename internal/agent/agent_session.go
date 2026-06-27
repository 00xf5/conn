package agent

import (
	"encoding/json"
	"io"
	"log"
	"runtime"
	"time"

	"connect/internal/inputproto"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

func (a *Agent) bindInputChannel(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		a.mu.Lock()
		w, h := a.capW, a.capH
		sess := a.activeSess
		a.mu.Unlock()
		if w > 0 && h > 0 {
			payload, _ := json.Marshal(map[string]any{"type": "screen", "w": w, "h": h})
			_ = dc.SendText(string(payload))
		}
		if sess != "" {
			go a.hostStatsLoop(dc, sess)
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			a.handleViewerJSON(msg.Data, dc)
			return
		}
		a.handleInput(msg.Data)
	})
}

func (a *Agent) handleViewerJSON(data []byte, dc *webrtc.DataChannel) {
	var hdr struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &hdr); err != nil {
		return
	}
	switch hdr.Type {
	case "viewer":
		a.handleStatsJSON(data)
	case "control":
		a.handleControl(data, dc)
	}
}

func (a *Agent) handleStatsJSON(data []byte) {
	var hdr struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &hdr); err != nil {
		return
	}
	if hdr.Type != "" && hdr.Type != "viewer" {
		return
	}
	var stats struct {
		PacketLoss float64 `json:"packetLoss"`
		RTT        float64 `json:"rtt"`
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		return
	}
	a.mu.Lock()
	enc := a.enc
	a.mu.Unlock()
	if enc == nil {
		return
	}
	kbps := a.cfg.BitrateK
	if stats.PacketLoss > 0.05 {
		kbps = int(float64(kbps) * 0.85)
	} else if stats.PacketLoss < 0.01 && stats.RTT < 100 {
		kbps = int(float64(kbps) * 1.1)
	}
	kbps = ProfileFromConfig(a.cfg).ClampBitrate(kbps)
	_ = enc.SetBitrate(kbps)
}

func (a *Agent) handleInput(data []byte) {
	if runtime.GOOS == "windows" {
		a.mu.Lock()
		w, h := a.capW, a.capH
		a.mu.Unlock()
		applyInputWindows(data, w, h)
		return
	}
	ev, err := inputproto.Decode(data)
	if err != nil {
		return
	}
	log.Printf("agent: input (non-windows stub): %+v", ev)
}

func (a *Agent) sessionAlive(sessionCode string, gen uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activeSess == sessionCode && a.sessGen == gen
}

func (a *Agent) pumpVideoTrack(track *webrtc.TrackLocalStaticSample, sessionCode string, gen uint64, gate <-chan struct{}) {
	select {
	case <-a.closed:
		return
	case <-gate:
	}
	prof := ProfileFromConfig(a.cfg)
	metrics := newSessionPerf(sessionCode)
	log.Printf("agent: video pump started (session %s, %dx%d @ %dfps)", sessionCode, prof.Width, prof.Height, prof.FPS)

	frameDur := prof.FrameDuration()
	latest := make(chan videoFrame, 1)
	go a.ingestVideoFrames(sessionCode, gen, latest)

	stall := time.NewTimer(prof.StallTimeout)
	defer stall.Stop()

	var sent int
	lastSend := time.Now()

	for {
		if !a.sessionAlive(sessionCode, gen) {
			return
		}
		select {
		case <-a.closed:
			return
		case <-stall.C:
			log.Printf("agent: video stalled (session %s)", sessionCode)
			a.endSession(sessionCode, gen)
			return
		case frame := <-latest:
			if !stall.Stop() {
				select {
				case <-stall.C:
				default:
				}
			}
			stall.Reset(prof.StallTimeout)

			if !a.sessionAlive(sessionCode, gen) {
				return
			}
			if len(frame.Data) == 0 {
				continue
			}

			if wait := time.Until(lastSend.Add(frameDur)); wait > 0 {
				time.Sleep(wait)
			}
			if err := track.WriteSample(media.Sample{
				Data:     frame.Data,
				Duration: frameDur,
			}); err != nil {
				log.Printf("agent: write sample: %v", err)
				return
			}
			lastSend = time.Now()
			sent++
			metrics.noteSent()
			if sent <= 3 || sent%120 == 0 {
				log.Printf("agent: sent %d video samples (key=%v, %d bytes)", sent, frame.KeyFrame, len(frame.Data))
			}
		}
	}
}

func (a *Agent) endSession(sessionCode string, gen uint64) {
	a.mu.Lock()
	if a.activeSess != sessionCode || a.sessGen != gen {
		a.mu.Unlock()
		return
	}
	a.closePeerLocked()
	a.mu.Unlock()
	go a.startWarmEncoder()
}

func (a *Agent) ingestVideoFrames(sessionCode string, gen uint64, out chan videoFrame) {
	for {
		if !a.sessionAlive(sessionCode, gen) {
			return
		}

		a.mu.Lock()
		enc := a.enc
		a.mu.Unlock()
		if enc == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		frame, err := enc.ReadFrame()
		if err != nil {
			if err != io.EOF {
				log.Printf("agent: frame read: %v", err)
			} else {
				log.Printf("agent: encoder EOF (session %s)", sessionCode)
			}
			a.endSession(sessionCode, gen)
			return
		}
		if len(frame.Data) == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		select {
		case out <- frame:
		default:
			select {
			case <-out:
			default:
			}
			select {
			case out <- frame:
			default:
			}
		}
	}
}

func (a *Agent) openVideoGate() {
	a.mu.Lock()
	gate := a.videoGate
	a.mu.Unlock()
	if gate == nil {
		return
	}
	select {
	case <-gate:
	default:
		close(gate)
	}
}

func (a *Agent) handleAnswer(payload json.RawMessage) {
	a.mu.Lock()
	pc := a.pc
	a.mu.Unlock()
	if pc == nil {
		log.Printf("agent: answer ignored (no peer connection)")
		return
	}
	var ans webrtc.SessionDescription
	if err := json.Unmarshal(payload, &ans); err != nil {
		log.Printf("agent: bad answer: %v", err)
		return
	}
	if err := pc.SetRemoteDescription(ans); err != nil {
		log.Printf("agent: set remote answer: %v", err)
		return
	}
	a.flushPendingICE(pc)
	log.Printf("agent: remote answer applied")
	a.openVideoGate()
}

func (a *Agent) handleICE(payload json.RawMessage) {
	var init webrtc.ICECandidateInit
	if err := json.Unmarshal(payload, &init); err != nil {
		log.Printf("agent: bad ICE: %v", err)
		return
	}
	a.mu.Lock()
	pc := a.pc
	if pc == nil {
		a.pendingICE = append(a.pendingICE, init)
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	if err := pc.AddICECandidate(init); err != nil {
		log.Printf("agent: add ICE candidate: %v", err)
	}
}

func (a *Agent) flushPendingICE(pc *webrtc.PeerConnection) {
	a.mu.Lock()
	pending := a.pendingICE
	a.pendingICE = nil
	a.mu.Unlock()
	for _, init := range pending {
		if err := pc.AddICECandidate(init); err != nil {
			log.Printf("agent: flush ICE candidate: %v", err)
		}
	}
}

func (a *Agent) closePeer() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closePeerLocked()
}

func (a *Agent) closePeerLocked() {
	a.sessGen++
	if a.pc != nil {
		_ = a.pc.Close()
		a.pc = nil
	}
	if a.enc != nil {
		_ = a.enc.Close()
		a.enc = nil
	}
	a.vtrack = nil
	a.videoGate = nil
	a.pendingICE = nil
	a.capW = 0
	a.capH = 0
	a.activeSess = ""
}
