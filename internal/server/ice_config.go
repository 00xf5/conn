package server

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/pion/turn/v4"
)

const defaultSTUNURLs = "stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302"

// ICEConfig holds STUN/TURN sources for WebRTC clients.
type ICEConfig struct {
	DefaultSTUN []string
	ExternalTURNURL    string
	ExternalTURNSecret string
}

func LoadICEConfigFromEnv() ICEConfig {
	cfg := ICEConfig{
		DefaultSTUN: splitCSV(os.Getenv("CONNECT_STUN_URLS")),
	}
	if len(cfg.DefaultSTUN) == 0 {
		cfg.DefaultSTUN = splitCSV(defaultSTUNURLs)
	}
	cfg.ExternalTURNURL = strings.TrimSpace(os.Getenv("CONNECT_TURN_URL"))
	cfg.ExternalTURNSecret = strings.TrimSpace(os.Getenv("CONNECT_TURN_SECRET"))
	return cfg
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (cfg ICEConfig) buildExternalTURN() (ICEServer, bool) {
	if cfg.ExternalTURNURL == "" || cfg.ExternalTURNSecret == "" {
		return ICEServer{}, false
	}
	user, cred, err := turn.GenerateLongTermTURNRESTCredentials(
		cfg.ExternalTURNSecret, "connect", turnCredLifetime,
	)
	if err != nil {
		return ICEServer{}, false
	}
	return ICEServer{
		URLs:       []string{cfg.ExternalTURNURL},
		Username:   user,
		Credential: cred,
	}, true
}

func stunServers(urls []string) []ICEServer {
	if len(urls) == 0 {
		return nil
	}
	return []ICEServer{{URLs: urls}}
}

// ParseICEServersJSON parses CONNECT_ICE_SERVERS (full WebRTC iceServers array JSON).
func ParseICEServersJSON(raw string) ([]ICEServer, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var wrap struct {
		ICEServers []ICEServer `json:"iceServers"`
	}
	if err := json.Unmarshal([]byte(raw), &wrap); err != nil {
		return nil, err
	}
	return wrap.ICEServers, nil
}
