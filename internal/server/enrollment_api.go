package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"connect/internal/auth"
	"connect/internal/store"
)

const (
	defaultEnrollmentTTLHours = 7 * 24 // 7 days
	minEnrollmentTTLHours     = 1
	maxEnrollmentTTLHours     = 90 * 24 // 90 days
)

func enrollmentTTL(hours int) time.Duration {
	if hours <= 0 {
		hours = defaultEnrollmentTTLHours
	}
	if hours < minEnrollmentTTLHours {
		hours = minEnrollmentTTLHours
	}
	if hours > maxEnrollmentTTLHours {
		hours = maxEnrollmentTTLHours
	}
	return time.Duration(hours) * time.Hour
}

func (s *Server) issueEnrollment(w http.ResponseWriter, r *http.Request, tenantID, label string, ttlHours int) {
	code, err := auth.GenerateEnrollmentCode()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hash, err := auth.HashEnrollmentCode(code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ttl := enrollmentTTL(ttlHours)
	exp := time.Now().UTC().Add(ttl)
	rec, err := s.db.CreateEnrollment(tenantID, label, hash, &exp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	host := r.Host
	if host == "" {
		host = "HOST"
	}
	writeJSON(w, map[string]any{
		"enrollment":     rec,
		"enrollmentCode": code, // once
		"expiresAt":      exp,
		"ttlHours":       int(ttl / time.Hour),
		"agentHint":      "connect-agent.exe -server wss://" + host + "/ws -enroll " + code,
	})
}

func (s *Server) handleAdminTenantSubroutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/tenants/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	tenantID, kind := parts[0], parts[1]
	switch kind {
	case "access-accounts":
		s.handleAdminAccessAccountsForTenant(w, r, tenantID)
	case "enrollments":
		s.handleEnrollmentsForTenant(w, r, tenantID, true)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAdminAccessAccountsForTenant(w http.ResponseWriter, r *http.Request, tenantID string) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.db.ListAccessAccounts(tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []store.AccessAccount{}
		}
		writeJSON(w, list)
	case http.MethodPost:
		var body struct {
			Label string `json:"label"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		code, err := auth.GenerateAccessCode()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hash, err := auth.HashAccessCode(code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		acc, err := s.db.CreateAccessAccount(tenantID, body.Label, hash, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{
			"account":    acc,
			"accessCode": code,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEnrollmentsForTenant(w http.ResponseWriter, r *http.Request, tenantID string, _ bool) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.db.ListEnrollments(tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []store.EnrollmentCode{}
		}
		writeJSON(w, list)
	case http.MethodPost:
		var body struct {
			Label    string `json:"label"`
			TTLHours int    `json:"ttlHours"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.issueEnrollment(w, r, tenantID, body.Label, body.TTLHours)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminEnrollmentRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/enrollments/")
	id = strings.TrimSuffix(id, "/revoke")
	id = strings.Trim(id, "/")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.db.RevokeEnrollment(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// Tech: list/issue enrollments for own tenant.
func (s *Server) handleTechEnrollments(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	s.handleEnrollmentsForTenant(w, r, claims.TenantID, false)
}

func (s *Server) handleTechEnrollmentRevoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/enrollments/")
	id = strings.TrimSuffix(id, "/revoke")
	id = strings.Trim(id, "/")
	list, err := s.db.ListEnrollments(claims.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	found := false
	for _, e := range list {
		if e.ID == id {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := s.db.RevokeEnrollment(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// Agent enrolls with a one-time code (no tech cookie).
func (s *Server) handleAgentEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.redeemLimit.allow("enroll:" + clientKey(r)) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var body struct {
		EnrollmentCode string `json:"enrollmentCode"`
		DeviceID       string `json:"deviceId"`
		Hostname       string `json:"hostname"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	code := strings.TrimSpace(body.EnrollmentCode)
	deviceID := strings.TrimSpace(body.DeviceID)
	if code == "" || deviceID == "" {
		http.Error(w, "enrollmentCode and deviceId required", http.StatusBadRequest)
		return
	}
	pending, err := s.db.ListPendingEnrollments()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var matched *store.EnrollmentCode
	for i := range pending {
		e := &pending[i]
		if e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt) {
			continue
		}
		if auth.CheckEnrollmentCode(e.CodeHash, code) {
			matched = e
			break
		}
	}
	if matched == nil {
		http.Error(w, "invalid enrollment code", http.StatusUnauthorized)
		return
	}
	rec, err := s.db.RedeemEnrollment(matched.ID, deviceID, body.Hostname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"ok":         true,
		"tenantId":   rec.TenantID,
		"tenantName": rec.TenantName,
		"deviceId":   deviceID,
	})
}
