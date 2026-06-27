package server

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pion/turn/v4"
)

const turnCredLifetime = 24 * time.Hour

// ICEServer is the JSON shape consumed by browsers and the agent.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

func publicHostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "127.0.0.1"
	}
	host := u.Hostname()
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

func (s *Server) ICEServers() []ICEServer {
	if len(s.cfg.OverrideICEServers) > 0 {
		return append([]ICEServer(nil), s.cfg.OverrideICEServers...)
	}

	var servers []ICEServer

	if ext, ok := s.cfg.ICE.buildExternalTURN(); ok {
		servers = append(servers, ext)
	}

	if s.turnSrv != nil && s.turnSecret != "" {
		host := s.cfg.PublicHost
		if host == "" {
			host = publicHostFromURL(s.cfg.PublicURL)
		}
		turnPort := s.cfg.TURNPort
		if turnPort <= 0 {
			turnPort = 3478
		}
		stunURL := fmt.Sprintf("stun:%s:%d", host, turnPort)
		servers = append(servers, ICEServer{URLs: []string{stunURL}})

		user, cred, err := turn.GenerateLongTermTURNRESTCredentials(s.turnSecret, "connect", turnCredLifetime)
		if err == nil {
			turnUDP := fmt.Sprintf("turn:%s:%d?transport=udp", host, turnPort)
			servers = append(servers, ICEServer{
				URLs:       []string{turnUDP},
				Username:   user,
				Credential: cred,
			})
		}
	}

	if len(servers) == 0 {
		servers = append(servers, stunServers(s.cfg.ICE.DefaultSTUN)...)
	} else if s.turnSrv == nil {
		// Cloud / no embedded TURN: always include public STUN as well.
		servers = append(servers, stunServers(s.cfg.ICE.DefaultSTUN)...)
	}

	return servers
}

func (s *Server) handleICE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"iceServers": s.ICEServers()})
}

func hostFromListenAddr(addr string) string {
	if addr == "" {
		return ""
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	if h, _, err := netSplitHostPort(host); err == nil && h != "" {
		if h == "0.0.0.0" || h == "::" || h == "[::]" {
			return ""
		}
		return h
	}
	return strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
}

func netSplitHostPort(hostport string) (string, string, error) {
	// url.Parse and net.SplitHostPort need bracketed IPv6
	if strings.Contains(hostport, "[") {
		return splitBracketHostPort(hostport)
	}
	parts := strings.SplitN(hostport, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return hostport, "", nil
}

func splitBracketHostPort(hostport string) (string, string, error) {
	i := strings.LastIndex(hostport, ":")
	if i < 0 {
		return hostport, "", nil
	}
	host := strings.TrimPrefix(strings.TrimSuffix(hostport[:i], "]"), "[")
	return host, hostport[i+1:], nil
}

func detectLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ip := ipNet.IP.String()
			if strings.HasPrefix(ip, "169.254.") {
				continue
			}
			return ip
		}
	}
	return ""
}
