package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"connect/internal/auth"
	"connect/internal/rendezvous"
	"connect/internal/session"
	"connect/internal/signaling"
	"connect/internal/store"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

//go:embed web/*
var webFS embed.FS

type Config struct {
	Addr               string
	PublicURL          string
	PublicHost         string
	KeyPath            string
	DBPath             string
	AuthSecretPath     string
	AdminToken         string
	RequireTenant      bool
	StaticRoot         fs.FS
	TLSCert            string
	TLSKey             string
	TURNPort           int
	TURNSecret         string
	EnableTURN         bool
	ICE                ICEConfig
	OverrideICEServers []ICEServer
	AgentDir           string // directory containing agent.zip for host install
}

type Server struct {
	cfg         Config
	sessions    *session.Store
	registry    *rendezvous.Registry
	hub         *signaling.Hub
	keyPair     auth.KeyPair
	tokens      *auth.TokenSigner
	db          *store.DB
	adminToken  string
	loginLimit  *keyedLimiter
	redeemLimit *keyedLimiter
	upgrader    websocket.Upgrader
	turnSecret  string
	turnSrv     interface{ Close() error }
}

func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = ":8787"
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost" + cfg.Addr
	}
	if cfg.KeyPath == "" {
		cfg.KeyPath = "data/server.key"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "data/connect.db"
	}
	if cfg.AuthSecretPath == "" {
		cfg.AuthSecretPath = filepath.Join(filepath.Dir(cfg.KeyPath), "auth.secret")
	}
	if cfg.TURNPort <= 0 {
		cfg.TURNPort = 3478
	}
	if cfg.PublicHost == "" {
		cfg.PublicHost = publicHostFromURL(cfg.PublicURL)
	}
	kp, err := auth.LoadOrCreateKeyPair(cfg.KeyPath)
	if err != nil {
		return nil, err
	}
	tokens, err := auth.LoadOrCreateSecret(cfg.AuthSecretPath)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	cfg.StaticRoot = static

	s := &Server{
		cfg:         cfg,
		sessions:    session.NewStore(),
		registry:    rendezvous.NewRegistry(),
		hub:         signaling.NewHub(),
		keyPair:     kp,
		tokens:      tokens,
		db:          db,
		adminToken:  loadAdminToken(cfg.AdminToken),
		loginLimit:  newKeyedLimiter(20, time.Minute),
		redeemLimit: newKeyedLimiter(30, time.Minute),
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}

	if cfg.EnableTURN {
		secretPath := cfg.TURNSecret
		if secretPath == "" {
			secretPath = "data/turn.secret"
		}
		secret, err := loadOrCreateTURNSecret(secretPath)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		s.turnSecret = secret
		turnHost := cfg.PublicHost
		if turnHost == "" || turnHost == "localhost" || turnHost == "127.0.0.1" {
			turnHost = detectLANIP()
		}
		if turnHost != "" {
			ts, err := startTURN(turnHost, cfg.TURNPort, secret)
			if err != nil {
				log.Printf("connectd: TURN disabled: %v", err)
			} else {
				s.turnSrv = ts
				log.Printf("connectd: TURN/STUN on UDP :%d (relay via %s)", cfg.TURNPort, turnHost)
			}
		}
	}

	log.Printf("connectd: sqlite %s", cfg.DBPath)
	return s, nil
}

func (s *Server) PublicKey() string {
	return s.keyPair.PublicKeyBase64()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/ice", s.handleICE)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/me", s.handleMe)
	mux.HandleFunc("/api/auth/redeem", s.handleAuthRedeem)
	mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/admin/login", s.handleAdminLogin)
	mux.HandleFunc("/api/admin/logout", s.handleAdminLogout)
	mux.HandleFunc("/api/admin/me", s.handleAdminMe)
	mux.HandleFunc("/api/admin/tenants", s.handleAdminTenants)
	mux.HandleFunc("/api/admin/tenants/", s.handleAdminTenantSubroutes)
	mux.HandleFunc("/api/admin/access-accounts/", s.handleAdminRevokeAccess)
	mux.HandleFunc("/api/admin/enrollments/", s.handleAdminEnrollmentRevoke)
	mux.HandleFunc("/api/admin/agents", s.handleAdminAgents)
	mux.HandleFunc("/api/admin/agent-package", s.handleAdminAgentPackage)
	mux.HandleFunc("/api/enrollments", s.handleTechEnrollments)
	mux.HandleFunc("/api/enrollments/", s.handleTechEnrollmentRevoke)
	mux.HandleFunc("/api/agent/enroll", s.handleAgentEnroll)
	mux.HandleFunc("/api/agent/package", s.handleAgentPackageInfo)
	mux.HandleFunc("/download/agent.zip", s.handleDownloadAgent)
	mux.HandleFunc("/download/setup.cmd", s.handleDownloadSetupCmd)
	mux.HandleFunc("/install", s.handleInstallPage)
	mux.HandleFunc("/install.ps1", s.handleInstallScript)
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/v/", s.handleViewer)
	mux.Handle("/", http.FileServer(http.FS(s.cfg.StaticRoot)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok":            true,
		"publicKey":     s.PublicKey(),
		"agents":        len(s.registry.List()),
		"turnEmbedded":  s.turnSrv != nil,
		"turnExternal":  s.cfg.ICE.ExternalTURNURL != "" && s.cfg.ICE.ExternalTURNSecret != "",
		"iceServers":    len(s.ICEServers()),
		"publicUrl":     s.cfg.PublicURL,
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			DeviceID string `json:"deviceId"`
			Mode     string `json:"mode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		agents := s.registry.ListByTenant(claims.TenantID)
		if body.DeviceID == "" {
			if len(agents) > 0 {
				body.DeviceID = agents[0].DeviceID
			}
		}
		if body.DeviceID == "" {
			http.Error(w, "no agent online", http.StatusBadRequest)
			return
		}
		if !s.deviceInTenant(body.DeviceID, claims.TenantID) {
			http.Error(w, "device not in tenant", http.StatusForbidden)
			return
		}
		sess, err := s.sessions.CreateMode(body.DeviceID, 30*time.Minute, body.Mode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"code":     sess.Code,
			"deviceId": sess.DeviceID,
			"mode":     sess.Mode,
			"viewer":   sess.ViewerURL(s.cfg.PublicURL),
			"expires":  sess.ExpiresAt,
		})
	case http.MethodDelete:
		code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
		if code == "" {
			http.Error(w, "code required", http.StatusBadRequest)
			return
		}
		if sess, ok := s.sessions.Get(code); ok && !s.deviceInTenant(sess.DeviceID, claims.TenantID) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.sessions.Delete(code)
		writeJSON(w, map[string]any{"ok": true, "code": code})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	writeJSON(w, s.registry.ListByTenant(claims.TenantID))
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	all := s.sessions.List()
	out := make([]*session.Session, 0, len(all))
	for _, sess := range all {
		if s.deviceInTenant(sess.DeviceID, claims.TenantID) {
			out = append(out, sess)
		}
	}
	writeJSON(w, out)
}

func (s *Server) deviceInTenant(deviceID, tenantID string) bool {
	if deviceID == "" || tenantID == "" {
		return false
	}
	if a, ok := s.registry.Get(deviceID); ok && a.TenantID == tenantID {
		return true
	}
	if b, err := s.db.GetAgentBinding(deviceID); err == nil && b.TenantID == tenantID {
		return true
	}
	return false
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	role := signaling.Role(r.URL.Query().Get("role"))
	sessionCode := r.URL.Query().Get("session")
	deviceID := r.URL.Query().Get("deviceId")
	hostname := r.URL.Query().Get("hostname")
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenantId"))

	if role != signaling.RoleAgent && role != signaling.RoleViewer {
		http.Error(w, "role must be agent or viewer", http.StatusBadRequest)
		return
	}
	if role == signaling.RoleAgent && deviceID == "" {
		deviceID = uuid.NewString()
	}
	if role == signaling.RoleViewer && sessionCode == "" {
		http.Error(w, "session required for viewer", http.StatusBadRequest)
		return
	}
	sessionCode = strings.ToUpper(strings.TrimSpace(sessionCode))
	if role == signaling.RoleViewer {
		if _, ok := s.sessions.Get(sessionCode); !ok {
			http.Error(w, "invalid or expired session", http.StatusNotFound)
			return
		}
	}
	if role == signaling.RoleAgent {
		if tenantID == "" {
			if b, err := s.db.GetAgentBinding(deviceID); err == nil {
				tenantID = b.TenantID
			}
		}
		if s.cfg.RequireTenant && tenantID == "" {
			http.Error(w, "tenantId required", http.StatusForbidden)
			return
		}
		if tenantID != "" {
			if _, err := s.db.GetTenant(tenantID); err != nil {
				http.Error(w, "unknown tenant", http.StatusForbidden)
				return
			}
			_ = s.db.UpsertAgentBinding(deviceID, tenantID, hostname)
		}
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	peer := signaling.NewPeer(role, sessionCode, deviceID, conn)
	peer.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	peer.Conn.SetPongHandler(func(string) error {
		return peer.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	go peer.WritePump()
	go peer.PingPump()

	if role == signaling.RoleAgent {
		s.hub.RegisterAgent(deviceID, peer)
		s.registry.Register(rendezvous.AgentInfo{
			DeviceID: deviceID,
			TenantID: tenantID,
			Hostname: hostname,
		})
		_ = peer.Send(signaling.Message{
			Type:     "registered",
			DeviceID: deviceID,
			Payload: mustRaw(map[string]any{
				"publicKey":  s.PublicKey(),
				"iceServers": s.ICEServers(),
				"tenantId":   tenantID,
			}),
		})
	} else {
		other := s.hub.JoinSession(sessionCode, peer)
		_ = peer.Send(signaling.Message{Type: "joined", Session: sessionCode})
		if other != nil && other.Role == signaling.RoleAgent {
			_ = other.Send(signaling.Message{Type: "peer-joined", From: role, Session: sessionCode})
			_ = peer.Send(signaling.Message{Type: "peer-present", From: other.Role, Session: sessionCode})
		} else if other != nil {
			_ = other.Send(signaling.Message{Type: "peer-joined", From: role, Session: sessionCode})
			_ = peer.Send(signaling.Message{Type: "peer-present", From: other.Role, Session: sessionCode})
		}
		s.notifyAgentIncomingViewer(sessionCode, peer)
	}

	s.readLoop(peer)
}

func (s *Server) notifyAgentIncomingViewer(sessionCode string, viewer *signaling.Peer) {
	sess, ok := s.sessions.Get(sessionCode)
	if !ok || sess.DeviceID == "" {
		return
	}
	agentPeer := s.hub.AgentPeer(sess.DeviceID)
	if agentPeer == nil {
		_ = viewer.Send(signaling.Message{Type: "no-host", Session: sessionCode})
		return
	}
	s.hub.JoinSession(sessionCode, agentPeer)
	mode := "full"
	if sess.Mode == "audio" {
		mode = "audio"
	}
	_ = agentPeer.Send(signaling.Message{
		Type:    "incoming-viewer",
		Session: sessionCode,
		From:    signaling.RoleViewer,
		Payload: mustRaw(map[string]any{"mode": mode}),
	})
	_ = viewer.Send(signaling.Message{Type: "peer-present", From: signaling.RoleAgent, Session: sessionCode})
}

func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.cfg.StaticRoot, "viewer/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) readLoop(peer *signaling.Peer) {
	defer func() {
		sessCode := peer.Session
		role := peer.Role
		s.hub.Unregister(peer)
		if role == signaling.RoleAgent {
			s.registry.Remove(peer.DeviceID)
		}
		// Viewer leave ends the Access ticket so Active Sessions does not grow forever.
		if role == signaling.RoleViewer && sessCode != "" {
			s.sessions.Delete(sessCode)
		}
		_ = peer.Conn.Close()
	}()

	for {
		_, raw, err := peer.Conn.ReadMessage()
		if err != nil {
			return
		}
		var msg signaling.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		msg.From = peer.Role
		if msg.Session == "" {
			msg.Session = peer.Session
		}
		switch msg.Type {
		case "heartbeat":
			if peer.Role == signaling.RoleAgent {
				level := parseAudioLevel(msg.Payload)
				s.registry.HeartbeatLevel(peer.DeviceID, level)
				// Enroll can finish after the agent already connected (installer race).
				if a, ok := s.registry.Get(peer.DeviceID); ok && a.TenantID == "" {
					if b, err := s.db.GetAgentBinding(peer.DeviceID); err == nil && b.TenantID != "" {
						s.registry.SetTenant(peer.DeviceID, b.TenantID, b.Hostname)
					}
				}
			}
		case "audio_level":
			if peer.Role == signaling.RoleAgent {
				level := parseAudioLevel(msg.Payload)
				if level >= 0 {
					s.registry.HeartbeatLevel(peer.DeviceID, level)
				}
			}
		case "offer", "answer", "ice", "stats":
			if msg.Session != "" {
				out, _ := json.Marshal(msg)
				s.hub.Relay(msg.Session, peer.Role, out)
			}
		case "request-host":
			if peer.Role == signaling.RoleViewer && msg.Session != "" {
				s.notifyAgentIncomingViewer(strings.ToUpper(strings.TrimSpace(msg.Session)), peer)
			}
		default:
			log.Printf("signaling: unknown type %q from %s", msg.Type, peer.Role)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parseAudioLevel(payload json.RawMessage) float64 {
	if len(payload) == 0 {
		return -1
	}
	var p struct {
		Level float64 `json:"level"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return -1
	}
	if p.Level < 0 {
		return 0
	}
	if p.Level > 1 {
		return 1
	}
	return p.Level
}

func mustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// ViewerPath returns /v/{code} path helper for redirects.
func ViewerPath(code string) string {
	return "/v/" + strings.ToUpper(code)
}
