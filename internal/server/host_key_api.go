package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"connect/internal/auth"
)

// ensureHostKey mints a permanent Host GUI key if the binding has none.
// Safe to call repeatedly; never overwrites an existing key.
func (s *Server) ensureHostKey(deviceID string) (string, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", nil
	}
	plain, err := s.db.GetHostKeyPlain(deviceID)
	if err != nil {
		return "", err
	}
	if plain != "" {
		return plain, nil
	}
	code, err := auth.GenerateHostKey()
	if err != nil {
		return "", err
	}
	hash, err := auth.HashHostKey(code)
	if err != nil {
		return "", err
	}
	if err := s.db.SetHostKey(deviceID, hash, code); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Server) rotateHostKey(deviceID string) (string, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", errBadRequest("deviceId required")
	}
	if _, err := s.db.GetAgentBinding(deviceID); err != nil {
		return "", err
	}
	code, err := auth.GenerateHostKey()
	if err != nil {
		return "", err
	}
	hash, err := auth.HashHostKey(code)
	if err != nil {
		return "", err
	}
	if err := s.db.SetHostKey(deviceID, hash, code); err != nil {
		return "", err
	}
	return code, nil
}

type badRequestError string

func (e badRequestError) Error() string { return string(e) }

func errBadRequest(msg string) error { return badRequestError(msg) }

// handleAgentHostKeyVerify unlocks the Host GUI (rate-limited, no cookie).
// POST { deviceId, hostKey } → { ok: true }
func (s *Server) handleAgentHostKeyVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.hostKeyLimit.allow(clientKey(r)) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var body struct {
		DeviceID string `json:"deviceId"`
		HostKey  string `json:"hostKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	deviceID := strings.TrimSpace(body.DeviceID)
	if deviceID == "" || strings.TrimSpace(body.HostKey) == "" {
		http.Error(w, "deviceId and hostKey required", http.StatusBadRequest)
		return
	}
	hash, err := s.db.GetHostKeyHash(deviceID)
	if err != nil || hash == "" {
		http.Error(w, "invalid host key", http.StatusUnauthorized)
		return
	}
	if !auth.CheckHostKey(hash, body.HostKey) {
		http.Error(w, "invalid host key", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "deviceId": deviceID})
}

// handleTechAgentSubroutes: /api/agents/{deviceId}/host-key/rotate
func (s *Server) handleTechAgentSubroutes(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	deviceID := parts[0]
	if !s.deviceInTenant(deviceID, claims.TenantID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if len(parts) == 3 && parts[1] == "host-key" && parts[2] == "rotate" && r.Method == http.MethodPost {
		code, err := s.rotateHostKey(deviceID)
		if err != nil {
			if _, ok := err.(badRequestError); ok {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "deviceId": deviceID, "hostKey": code})
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

// handleAdminAgentSubroutes: /api/admin/agents/{deviceId}/host-key/rotate
func (s *Server) handleAdminAgentSubroutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/agents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	deviceID := parts[0]
	if len(parts) == 3 && parts[1] == "host-key" && parts[2] == "rotate" && r.Method == http.MethodPost {
		code, err := s.rotateHostKey(deviceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "deviceId": deviceID, "hostKey": code})
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}
