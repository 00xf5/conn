package rendezvous

import (
	"sync"
	"time"
)

// HostInventory is optional host telemetry from agent heartbeats.
// Omitted entirely until an agent sends it (Phase 3+).
type HostInventory struct {
	FQDN           string  `json:"fqdn,omitempty"`
	User           string  `json:"user,omitempty"`
	Domain         string  `json:"domain,omitempty"`
	OS             string  `json:"os,omitempty"`
	OSVersion      string  `json:"osVersion,omitempty"`
	Arch           string  `json:"arch,omitempty"`
	UptimeSec      uint64  `json:"uptimeSec,omitempty"`
	Manufacturer   string  `json:"manufacturer,omitempty"`
	Model          string  `json:"model,omitempty"`
	BIOS           string  `json:"bios,omitempty"`
	Serial         string  `json:"serial,omitempty"`
	UUID           string  `json:"uuid,omitempty"`
	CPU            string  `json:"cpu,omitempty"`
	CPUCores       int     `json:"cpuCores,omitempty"`
	CPUPct         float64 `json:"cpuPct,omitempty"`
	MemTotalGB     float64 `json:"memTotalGb,omitempty"`
	MemUsedGB      float64 `json:"memUsedGb,omitempty"`
	MemAvailGB     float64 `json:"memAvailGb,omitempty"`
	MemPct         float64 `json:"memPct,omitempty"`
	PagefileTotGB  float64 `json:"pagefileTotalGb,omitempty"`
	PagefileAvailGB float64 `json:"pagefileAvailGb,omitempty"`
	DiskVol        string  `json:"diskVol,omitempty"`
	DiskTotalGB    float64 `json:"diskTotalGb,omitempty"`
	DiskFreeGB     float64 `json:"diskFreeGb,omitempty"`
	IPv4           string  `json:"ipv4,omitempty"`
	IPv6           string  `json:"ipv6,omitempty"`
	MAC            string  `json:"mac,omitempty"`
	Adapter        string  `json:"adapter,omitempty"`
	Monitors       int     `json:"monitors,omitempty"`
	Resolution     string  `json:"resolution,omitempty"`
	FPS            int     `json:"fps,omitempty"`
	BitrateK       int     `json:"bitrateK,omitempty"`
	GOP            int     `json:"gop,omitempty"`
	Encoder        string  `json:"encoder,omitempty"`
	AgentVersion   string  `json:"agentVersion,omitempty"`
	ServerURL      string  `json:"serverUrl,omitempty"`
	Monitor        int     `json:"monitor"`
	SessionActive  *bool   `json:"sessionActive,omitempty"`
}

type AgentInfo struct {
	DeviceID   string         `json:"deviceId"`
	TenantID   string         `json:"tenantId,omitempty"`
	Hostname   string         `json:"hostname"`
	Online     bool           `json:"online"`
	Connected  time.Time      `json:"connected"`
	LastSeen   time.Time      `json:"lastSeen"`
	Encoder    string         `json:"encoder,omitempty"`
	Resolution string         `json:"resolution,omitempty"`
	AudioLevel float64        `json:"audioLevel,omitempty"`
	HostKey    string         `json:"hostKey,omitempty"` // permanent Host GUI unlock (tech copy)
	Inventory  *HostInventory `json:"inventory,omitempty"`
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
		if info.Inventory == nil && existing.Inventory != nil {
			info.Inventory = existing.Inventory
		}
		if info.Encoder == "" {
			info.Encoder = existing.Encoder
		}
		if info.Resolution == "" {
			info.Resolution = existing.Resolution
		}
	} else {
		info.Connected = now
	}
	info.LastSeen = now
	info.Online = true
	r.agents[info.DeviceID] = &info
}

func (r *Registry) Heartbeat(deviceID string) {
	r.HeartbeatLevel(deviceID, -1)
}

// HeartbeatLevel updates last-seen; if level >= 0, also stores audioLevel (0..1).
func (r *Registry) HeartbeatLevel(deviceID string, level float64) {
	r.ApplyHeartbeat(deviceID, level, nil)
}

// ApplyHeartbeat updates last-seen, optional audio level, and optional inventory.
// level < 0 skips audio; inv nil skips inventory. Old agents keep working.
func (r *Registry) ApplyHeartbeat(deviceID string, level float64, inv *HostInventory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.agents[deviceID]
	if !ok {
		return
	}
	a.LastSeen = time.Now()
	if level >= 0 {
		if level > 1 {
			level = 1
		}
		a.AudioLevel = level
	}
	if inv != nil {
		a.Inventory = inv
		if inv.Encoder != "" {
			a.Encoder = inv.Encoder
		}
		if inv.Resolution != "" {
			a.Resolution = inv.Resolution
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
