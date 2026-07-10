package agent

import (
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type Agent struct {
	cfg         Config
	conn        *websocket.Conn
	connWriteMu sync.Mutex
	mu          sync.Mutex
	pc         *webrtc.PeerConnection
	enc        videoEncoder
	closed     chan struct{}
	stopOnce   sync.Once
	pendingICE []webrtc.ICECandidateInit
	activeSess string
	sessGen    uint64
	sessStart  sync.Mutex
	state      string
	videoGate  chan struct{}
	capW       int
	capH       int
	vtrack     *webrtc.TrackLocalStaticSample
	atrack     *webrtc.TrackLocalStaticSample
	audio      *audioRuntime
	warmEnc    videoEncoder
	warmMu     sync.Mutex
	warming    bool
	iceServers []webrtc.ICEServer

	// Gentle adaptive bitrate (session-local; manual slider holds it off briefly).
	abrBitrateK   int
	abrLastAdjust time.Time
	abrHoldUntil  time.Time
}

func New(cfg Config) *Agent {
	if cfg.ServerURL == "" {
		cfg.ServerURL = "wss://localhost:8787/ws"
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = loadOrCreateDeviceID()
	}
	if cfg.Hostname == "" {
		cfg.Hostname, _ = os.Hostname()
	}
	cfg = NormalizeConfig(cfg)
	return &Agent{cfg: cfg, closed: make(chan struct{})}
}

func (a *Agent) DeviceID() string { return a.cfg.DeviceID }

func (a *Agent) Stop() {
	a.stopOnce.Do(func() {
		close(a.closed)
		a.mu.Lock()
		if a.conn != nil {
			_ = a.conn.Close()
			a.conn = nil
		}
		a.stopAudioLocked()
		a.mu.Unlock()
		a.closePeer()
		a.closeWarmEncoder()
		a.setState("offline", "-")
	})
}
