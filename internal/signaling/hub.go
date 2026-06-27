package signaling

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Role string

const (
	RoleAgent  Role = "agent"
	RoleViewer Role = "viewer"
)

type Message struct {
	Type      string          `json:"type"`
	Session   string          `json:"session,omitempty"`
	From      Role            `json:"from,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	DeviceID  string          `json:"deviceId,omitempty"`
}

type Peer struct {
	Role     Role
	Session  string
	DeviceID string
	Conn     *websocket.Conn
	outbox   chan []byte
	closeOnce sync.Once
}

func (p *Peer) shutdown() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		close(p.outbox)
	})
}

func NewPeer(role Role, session, deviceID string, conn *websocket.Conn) *Peer {
	return &Peer{
		Role:     role,
		Session:  session,
		DeviceID: deviceID,
		Conn:     conn,
		outbox:   make(chan []byte, 64),
	}
}

type Hub struct {
	mu      sync.RWMutex
	agents  map[string]*Peer // deviceID -> agent
	rooms   map[string]map[Role]*Peer
}

func NewHub() *Hub {
	return &Hub{
		agents: make(map[string]*Peer),
		rooms:  make(map[string]map[Role]*Peer),
	}
}

func (h *Hub) AgentPeer(deviceID string) *Peer {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.agents[deviceID]
}

func (h *Hub) RegisterAgent(deviceID string, p *Peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.agents[deviceID]; ok {
		old.shutdown()
		_ = old.Conn.Close()
	}
	h.agents[deviceID] = p
}

func (h *Hub) Unregister(p *Peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p.Role == RoleAgent {
		if cur, ok := h.agents[p.DeviceID]; ok && cur == p {
			delete(h.agents, p.DeviceID)
		}
	}
	if room, ok := h.rooms[p.Session]; ok {
		if cur, ok := room[p.Role]; ok && cur == p {
			delete(room, p.Role)
		}
		if len(room) == 0 {
			delete(h.rooms, p.Session)
		}
	}
	p.shutdown()
}

func (h *Hub) JoinSession(sessionCode string, p *Peer) *Peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, ok := h.rooms[sessionCode]
	if !ok {
		room = make(map[Role]*Peer)
		h.rooms[sessionCode] = room
	}
	if existing, ok := room[p.Role]; ok {
		existing.shutdown()
		_ = existing.Conn.Close()
	}
	room[p.Role] = p
	p.Session = sessionCode

	var other *Peer
	for role, peer := range room {
		if role != p.Role {
			other = peer
			break
		}
	}
	return other
}

func (h *Hub) AgentOnline(deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.agents[deviceID]
	return ok
}

func (h *Hub) OnlineAgents() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.agents))
	for id := range h.agents {
		out = append(out, id)
	}
	return out
}

func (h *Hub) SessionPeers(sessionCode string) (map[Role]*Peer, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room, ok := h.rooms[sessionCode]
	return room, ok
}

func (h *Hub) Relay(sessionCode string, from Role, raw []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room, ok := h.rooms[sessionCode]
	if !ok {
		return
	}
	for role, peer := range room {
		if role != from {
			select {
			case peer.outbox <- raw:
			default:
				log.Printf("signaling: drop message to %s (slow consumer)", role)
			}
			break
		}
	}
}

func (p *Peer) WritePump() {
	for msg := range p.outbox {
		if err := p.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (p *Peer) Send(msg Message) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case p.outbox <- raw:
		return nil
	default:
		return websocket.ErrCloseSent
	}
}
