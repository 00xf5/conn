package agent

import (
	"encoding/json"
	"io"
	"log"
	"runtime"
	"sync"
	"time"

	"connect/internal/captureenc"
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
		Mobile     bool    `json:"mobile"`
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		return
	}
	a.mu.Lock()
	enc := a.enc
	if _, native := enc.(*hostPipelineEncoder); native {
		// Native HW pipeline: bitrate tweaks on every stats tick stall encode.
		a.mu.Unlock()
		return
	}
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
	a.mu.Lock()
	sess := a.activeSess
	a.mu.Unlock()
	if sess != "" && (stats.PacketLoss > 0.02 || stats.RTT > 200 || stats.Mobile) {
		log.Printf("agent: viewer stats session=%s mobile=%t rtt=%.0fms loss=%.1f%%",
			sess, stats.Mobile, stats.RTT, stats.PacketLoss*100)
	}
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
	pipeline := ""
	a.mu.Lock()
	if a.enc != nil {
		pipeline = a.enc.Name()
	}
	a.mu.Unlock()
	metrics := newSessionPerf(sessionCode, pipeline)
	log.Printf("agent: video pump started (session %s, %dx%d @ %dfps)", sessionCode, prof.Width, prof.Height, prof.FPS)

	frameDur := prof.FrameDuration()
	slot := &latestVideoFrame{}
	go a.fillLatestVideoFrame(sessionCode, gen, slot, metrics)

	ticker := time.NewTicker(frameDur)
	defer ticker.Stop()
	stall := time.NewTimer(prof.StallTimeout)
	defer stall.Stop()

	var sent int
	var stalled bool
	sampleTS := time.Now()
	defer func() {
		metrics.logSummary(stalled)
	}()

	for {
		if !a.sessionAlive(sessionCode, gen) {
			return
		}
		select {
		case <-a.closed:
			return
		case <-stall.C:
			if !a.sessionAlive(sessionCode, gen) {
				return
			}
			stalled = true
			log.Printf("agent: video stalled (session %s)", sessionCode)
			a.endSession(sessionCode, gen)
			return
		case <-ticker.C:
			frame, ok := slot.take()
			if !ok || len(frame.Data) == 0 {
				continue
			}
			if !stall.Stop() {
				select {
				case <-stall.C:
				default:
				}
			}
			stall.Reset(prof.StallTimeout)

			if err := track.WriteSample(media.Sample{
				Data:      frame.Data,
				Duration:  frameDur,
				Timestamp: sampleTS,
			}); err != nil {
				log.Printf("agent: write sample: %v", err)
				return
			}
			sampleTS = sampleTS.Add(frameDur)
			sent++
			metrics.noteSent()
			if sent <= 3 || sent%120 == 0 {
				log.Printf("agent: sent %d video samples (key=%v, %d bytes)", sent, frame.KeyFrame, len(frame.Data))
			}
		}
	}
}

// latestVideoFrame holds the most recent encoded frame for the ticker pump.
type latestVideoFrame struct {
	mu    sync.Mutex
	frame videoFrame
	ok    bool
}

func (s *latestVideoFrame) store(frame videoFrame) {
	s.mu.Lock()
	s.frame = frame
	s.ok = true
	s.mu.Unlock()
}

func (s *latestVideoFrame) take() (videoFrame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frame, s.ok
}

func (a *Agent) fillLatestVideoFrame(sessionCode string, gen uint64, slot *latestVideoFrame, metrics *sessionPerf) {
	gotKeyframe := false
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
			continue
		}
		if !acceptVideoFrame(frame) {
			metrics.noteRejected()
			continue
		}
		if !gotKeyframe {
			if !frame.KeyFrame || len(frame.Data) < captureenc.MinKeyframeBytes ||
				!captureenc.ContainsNALType(frame.Data, 5) {
				metrics.noteSkippedNonKey()
				continue
			}
			gotKeyframe = true
			log.Printf("agent: session %s live keyframe ready (%d bytes)", sessionCode, len(frame.Data))
		}
		slot.store(frame)
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
	a.mu.Lock()
	sess := a.activeSess
	a.mu.Unlock()
	if sess != "" {
		a.setState("streaming", sess)
	}
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
