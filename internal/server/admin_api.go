package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"connect/internal/auth"
	"connect/internal/store"
)

type keyedLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
	limit   int
	window  time.Duration
}

func newKeyedLimiter(limit int, window time.Duration) *keyedLimiter {
	return &keyedLimiter{
		buckets: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

func (l *keyedLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.window)
	hits := l.buckets[key]
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.buckets[key] = kept
		return false
	}
	l.buckets[key] = append(kept, now)
	return true
}

func clientKey(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.loginLimit.allow(clientKey(r)) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.Token) == "" || body.Token != s.adminToken {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	tok, err := s.tokens.Issue(auth.TokenClaims{Role: auth.RoleAdmin}, 12*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setCookie(w, auth.CookieAdmin, tok, 12*time.Hour)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.clearCookie(w, auth.CookieAdmin)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAdminTenants(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.db.ListTenants()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []store.Tenant{}
		}
		writeJSON(w, list)
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		t, err := s.db.CreateTenant(body.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminAccessAccounts(w http.ResponseWriter, r *http.Request) {
	// Legacy name kept for clarity; routing is via handleAdminTenantSubroutes.
	s.handleAdminTenantSubroutes(w, r)
}

func (s *Server) handleAdminRevokeAccess(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/access-accounts/")
	id = strings.TrimSuffix(id, "/revoke")
	id = strings.Trim(id, "/")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.db.RevokeAccessAccount(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleAdminAgents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	online := s.registry.List()
	bindings, _ := s.db.ListAgentBindings()
	byDevice := map[string]store.AgentBinding{}
	for _, b := range bindings {
		byDevice[b.DeviceID] = b
	}
	type row struct {
		DeviceID  string    `json:"deviceId"`
		TenantID  string    `json:"tenantId,omitempty"`
		Hostname  string    `json:"hostname"`
		Online    bool      `json:"online"`
		LastSeen  time.Time `json:"lastSeen,omitempty"`
		Connected time.Time `json:"connected,omitempty"`
		HostKey   string    `json:"hostKey,omitempty"`
	}
	seen := map[string]bool{}
	out := make([]row, 0, len(online)+len(bindings))
	for _, a := range online {
		tid := a.TenantID
		if tid == "" {
			if b, ok := byDevice[a.DeviceID]; ok {
				tid = b.TenantID
			}
		}
		key, _ := s.ensureHostKey(a.DeviceID)
		out = append(out, row{
			DeviceID: a.DeviceID, TenantID: tid, Hostname: a.Hostname,
			Online: true, LastSeen: a.LastSeen, Connected: a.Connected, HostKey: key,
		})
		seen[a.DeviceID] = true
	}
	for _, b := range bindings {
		if seen[b.DeviceID] {
			continue
		}
		key, _ := s.ensureHostKey(b.DeviceID)
		out = append(out, row{
			DeviceID: b.DeviceID, TenantID: b.TenantID, Hostname: b.Hostname, Online: false, HostKey: key,
		})
	}
	writeJSON(w, out)
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	writeJSON(w, map[string]any{"role": "admin", "ok": true})
}
