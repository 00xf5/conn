package agent

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func (a *Agent) Run() error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("agent: panic recovered: %v", r)
		}
	}()
	for {
		select {
		case <-a.closed:
			return nil
		default:
		}
		if err := a.connectOnce(); err != nil {
			log.Printf("agent: disconnected: %v; retry in 3s", err)
			time.Sleep(3 * time.Second)
		}
	}
}

func (a *Agent) connectOnce() error {
	u, err := url.Parse(a.cfg.ServerURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("role", "agent")
	q.Set("deviceId", a.cfg.DeviceID)
	q.Set("hostname", a.cfg.Hostname)
	u.RawQuery = q.Encode()

	dialer := websocket.DefaultDialer
	if u.Scheme == "wss" && a.cfg.InsecureTLS {
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // LAN self-signed connectd cert
		}
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if a.conn == conn {
			a.conn = nil
		}
		a.mu.Unlock()
		conn.Close()
	}()

	log.Printf("agent: connected as %s (%s)", a.cfg.DeviceID, a.cfg.Hostname)
	a.setState("online", "-")

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.readLoop()
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			a.closePeer()
			a.setState("offline", "-")
			return fmt.Errorf("connection closed")
		case <-a.closed:
			return nil
		case <-ticker.C:
			_ = a.send(signalingEnvelope{Type: "heartbeat"})
		}
	}
}

func (a *Agent) readLoop() {
	for {
		_, raw, err := a.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg signalingEnvelope
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "registered":
			a.setICEServers(parseICEServers(msg.Payload))
			log.Printf("agent: registered with server")
			go a.preloadEncoder()
		case "incoming-viewer":
			if msg.Session != "" {
				go a.startSession(msg.Session)
			}
		case "answer":
			a.handleAnswer(msg.Payload)
		case "ice":
			a.handleICE(msg.Payload)
		}
	}
}

func (a *Agent) send(msg signalingEnvelope) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return a.conn.WriteMessage(websocket.TextMessage, raw)
}

func loadOrCreateDeviceID() string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		dir = "data"
	} else {
		dir = filepath.Join(dir, "Connect")
	}
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "device.id")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b))
	}
	id := uuid.NewString()
	_ = os.WriteFile(path, []byte(id), 0o600)
	return id
}
