package agent

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/pion/webrtc/v4"
)

// HostSnapshot is sent to the viewer over the input datachannel.
type HostSnapshot struct {
	Type        string            `json:"type"`
	Hostname    string            `json:"hostname"`
	CPU         float64           `json:"cpu"`
	CPUCores    int               `json:"cpuCores"`
	MemUsedGB   float64           `json:"memUsedGb"`
	MemTotalGB  float64           `json:"memTotalGb"`
	MemPct      float64           `json:"memPct"`
	DiskFreeGB  float64           `json:"diskFreeGb"`
	DiskTotalGB float64           `json:"diskTotalGb"`
	UptimeSec   uint64            `json:"uptimeSec"`
	Encoder     string            `json:"encoder"`
	FPS         int               `json:"fps"`
	BitrateK    int               `json:"bitrateK"`
	Processes   []ProcessSnapshot `json:"processes"`
}

func buildHostSnapshot(cfg Config, encoderName string) HostSnapshot {
	m := sampleHostMetrics()
	host := cfg.Hostname
	if host == "" {
		host = "host"
	}
	cores := m.CPUCores
	if cores <= 0 {
		cores = runtime.NumCPU()
	}
	return HostSnapshot{
		Type:        "host",
		Hostname:    host,
		CPU:         m.CPU,
		CPUCores:    cores,
		MemUsedGB:   m.MemUsedGB,
		MemTotalGB:  m.MemTotalGB,
		MemPct:      m.MemPct,
		DiskFreeGB:  m.DiskFreeGB,
		DiskTotalGB: m.DiskTotalGB,
		UptimeSec:   m.UptimeSec,
		Encoder:     encoderName,
		FPS:         cfg.FPS,
		BitrateK:    cfg.BitrateK,
		Processes:   m.Processes,
	}
}

func (a *Agent) hostStatsLoop(dc *webrtc.DataChannel, session string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.closed:
			return
		case <-ticker.C:
			a.mu.Lock()
			active := a.activeSess == session
			enc := a.enc
			cfg := a.cfg
			a.mu.Unlock()
			if !active {
				return
			}
			name := ""
			if enc != nil {
				name = enc.Name()
			}
			payload, err := json.Marshal(buildHostSnapshot(cfg, name))
			if err != nil {
				continue
			}
			if err := dc.SendText(string(payload)); err != nil {
				return
			}
		}
	}
}
