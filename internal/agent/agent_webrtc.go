package agent

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"connect/internal/captureenc"

	"github.com/pion/webrtc/v4"
)

func (a *Agent) startSession(sessionCode string) {
	a.sessStart.Lock()
	defer a.sessStart.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("agent: session panic: %v", r)
			a.mu.Lock()
			a.activeSess = ""
			a.mu.Unlock()
			go a.startWarmEncoder()
		}
	}()

	sessionCode = strings.ToUpper(strings.TrimSpace(sessionCode))
	t0 := time.Now()
	log.Printf("agent: session %s requested", sessionCode)

	a.mu.Lock()
	if a.activeSess == sessionCode && a.pc != nil {
		switch a.pc.ConnectionState() {
		case webrtc.PeerConnectionStateConnected, webrtc.PeerConnectionStateConnecting:
			log.Printf("agent: session %s new viewer — restarting WebRTC", sessionCode)
			a.closePeerLocked()
		}
	}
	if a.activeSess != "" && a.activeSess != sessionCode {
		a.closePeerLocked()
	}
	a.sessGen++
	gen := a.sessGen
	a.activeSess = sessionCode
	a.pendingICE = nil
	a.mu.Unlock()

	var enc videoEncoder
	var err error
	enc = a.takeWarmEncoder()
	if enc == nil {
		enc, err = openVideoEncoder(a.cfg)
	} else {
		err = nil
	}
	if enc == nil || err != nil {
		log.Printf("agent: no video encoder: %v", err)
		a.mu.Lock()
		a.activeSess = ""
		a.mu.Unlock()
		return
	}
	log.Printf("agent: encoder %s (ready_ms=%d)", enc.Name(), time.Since(t0).Milliseconds())
	if w, h := enc.CaptureSize(); w > 0 && h > 0 {
		a.mu.Lock()
		a.capW, a.capH = w, h
		a.mu.Unlock()
		log.Printf("agent: capture size %dx%d", w, h)
	}

	prof := ProfileFromConfig(a.cfg)
	requestEncoderKeyframe(enc)

	firstKF, err := readLiveKeyframe(enc, captureenc.MinKeyframeBytes, keyframeWaitTimeout(prof))
	if err != nil {
		log.Printf("agent: no keyframe for SDP: %v", err)
		_ = enc.Close()
		a.mu.Lock()
		a.activeSess = ""
		a.mu.Unlock()
		return
	}
	log.Printf("agent: H.264 profile-level-id=%s", spsProfileLevelID(firstKF.Data))
	// SDP consumed one IDR; start the wire stream from a fresh GOP.
	requestEncoderKeyframe(enc)

	log.Printf("agent: creating peer connection session=%s", sessionCode)
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: a.iceConfig(),
	})
	if err != nil {
		log.Printf("agent: peer connection failed: %v", err)
		_ = enc.Close()
		a.mu.Lock()
		a.activeSess = ""
		a.mu.Unlock()
		return
	}

	vtrack, err := webrtc.NewTrackLocalStaticSample(h264CodecCapabilityFromAnnexB(firstKF.Data), "video", "connect")
	if err != nil {
		log.Printf("agent: video track failed: %v", err)
		_ = enc.Close()
		_ = pc.Close()
		a.mu.Lock()
		a.activeSess = ""
		a.mu.Unlock()
		return
	}
	if _, err = pc.AddTrack(vtrack); err != nil {
		log.Printf("agent: add track failed: %v", err)
		_ = enc.Close()
		_ = pc.Close()
		a.mu.Lock()
		a.activeSess = ""
		a.mu.Unlock()
		return
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		payload, _ := json.Marshal(init)
		_ = a.send(signalingEnvelope{Type: "ice", Session: sessionCode, Payload: payload})
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("agent: WebRTC state %s (session %s)", state, sessionCode)
		switch state {
		case webrtc.PeerConnectionStateConnected:
			a.openVideoGate()
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateDisconnected:
			a.mu.Lock()
			if a.activeSess == sessionCode {
				a.closePeerLocked()
				a.state = "online"
			}
			a.mu.Unlock()
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		a.bindInputChannel(dc)
	})
	if dc, err := pc.CreateDataChannel("input", nil); err == nil {
		a.bindInputChannel(dc)
	}

	a.mu.Lock()
	a.enc = guardEncoder(enc)
	a.pc = pc
	a.vtrack = vtrack
	a.videoGate = make(chan struct{})
	videoGate := a.videoGate
	a.mu.Unlock()

	go a.pumpVideoTrack(vtrack, sessionCode, gen, videoGate)

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("agent: create offer failed: %v", err)
		a.mu.Lock()
		a.closePeerLocked()
		a.mu.Unlock()
		go a.startWarmEncoder()
		return
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		log.Printf("agent: set local description failed: %v", err)
		a.mu.Lock()
		a.closePeerLocked()
		a.mu.Unlock()
		go a.startWarmEncoder()
		return
	}
	payload, _ := json.Marshal(offer)
	_ = a.send(signalingEnvelope{Type: "offer", Session: sessionCode, Payload: payload})
	log.Printf("agent: WebRTC offer sent for session %s", sessionCode)
}
