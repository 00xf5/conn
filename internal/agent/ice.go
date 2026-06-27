package agent

import (
	"encoding/json"

	"github.com/pion/webrtc/v4"
)

type iceServerPayload struct {
	URLs       json.RawMessage `json:"urls"`
	Username   string          `json:"username"`
	Credential string          `json:"credential"`
}

func parseICEServers(payload json.RawMessage) []webrtc.ICEServer {
	var wrap struct {
		ICEServers []iceServerPayload `json:"iceServers"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &wrap) != nil {
		return nil
	}
	out := make([]webrtc.ICEServer, 0, len(wrap.ICEServers))
	for _, s := range wrap.ICEServers {
		urls := parseICEURLs(s.URLs)
		if len(urls) == 0 {
			continue
		}
		out = append(out, webrtc.ICEServer{
			URLs:       urls,
			Username:   s.Username,
			Credential: s.Credential,
		})
	}
	return out
}

func parseICEURLs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}

func (a *Agent) iceConfig() []webrtc.ICEServer {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.iceServers) > 0 {
		return a.iceServers
	}
	return []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}
}

func (a *Agent) setICEServers(servers []webrtc.ICEServer) {
	if len(servers) == 0 {
		return
	}
	a.mu.Lock()
	a.iceServers = servers
	a.mu.Unlock()
}
