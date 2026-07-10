package rendezvous

import (
	"sync"
	"time"
)

type AgentInfo struct {
	DeviceID   string    `json:"deviceId"`
	TenantID   string    `json:"tenantId,omitempty"`
	Hostname   string    `json:"hostname"`
	Connected  time.Time `json:"connected"`
	LastSeen   time.Time `json:"lastSeen"`
	Encoder    string    `json:"encoder,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	AudioLevel float64   `json:"audioLevel,omitempty"`
}

type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
}

func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]*AgentInfo)}
}

func (r *Registry) Register(info AgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if existing, ok := r.agents[info.DeviceID]; ok {
		info.Connected = existing.Connected
		if info.AudioLevel == 0 && existing.AudioLevel > 0 {
			info.AudioLevel = existing.AudioLevel
		}
	} else {
		info.Connected = now
	}
	info.LastSeen = now
	r.agents[info.DeviceID] = &info
}

func (r *Registry) Heartbeat(deviceID string) {
	r.HeartbeatLevel(deviceID, -1)
}

// HeartbeatLevel updates last-seen; if level >= 0, also stores audioLevel (0..1).
func (r *Registry) HeartbeatLevel(deviceID string, level float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[deviceID]; ok {
		a.LastSeen = time.Now()
		if level >= 0 {
			if level > 1 {
				level = 1
			}
			a.AudioLevel = level
		}
	}
}

// SetTenant updates tenant/hostname for an online agent (e.g. enroll finished after connect).
func (r *Registry) SetTenant(deviceID, tenantID, hostname string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.agents[deviceID]
	if !ok {
		return
	}
	if tenantID != "" {
		a.TenantID = tenantID
	}
	if hostname != "" {
		a.Hostname = hostname
	}
	a.LastSeen = time.Now()
}

func (r *Registry) Remove(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, deviceID)
}

func (r *Registry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, *a)
	}
	return out
}

func (r *Registry) Get(deviceID string) (AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[deviceID]
	if !ok {
		return AgentInfo{}, false
	}
	return *a, true
}

func (r *Registry) ListByTenant(tenantID string) []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentInfo, 0)
	for _, a := range r.agents {
		if a.TenantID == tenantID {
			out = append(out, *a)
		}
	}
	return out
}
