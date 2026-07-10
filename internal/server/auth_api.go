package server

import (
	"encoding/json"
	"net/http"
	"time"

	"connect/internal/auth"
	"connect/internal/store"
)

func (s *Server) handleAuthRedeem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.redeemLimit.allow(clientKey(r)) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var body struct {
		AccessCode string `json:"accessCode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	code := auth.NormalizeAccessCode(body.AccessCode)
	if len(code) < 12 {
		http.Error(w, "invalid access code", http.StatusBadRequest)
		return
	}
	accounts, err := s.db.ListRedeemableAccounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var matched *store.AccessAccount
	for i := range accounts {
		a := &accounts[i]
		if a.ExpiresAt != nil && time.Now().After(*a.ExpiresAt) {
			continue
		}
		if auth.CheckAccessCode(a.CodeHash, code) {
			matched = a
			break
		}
	}
	if matched == nil {
		http.Error(w, "invalid access code", http.StatusUnauthorized)
		return
	}
	if err := s.db.MarkAccessRedeemed(matched.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tok, err := s.tokens.Issue(auth.TokenClaims{
		Role:      auth.RoleTech,
		AccountID: matched.ID,
		TenantID:  matched.TenantID,
	}, 12*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setCookie(w, auth.CookieTech, tok, 12*time.Hour)
	ten, _ := s.db.GetTenant(matched.TenantID)
	writeJSON(w, map[string]any{
		"ok":         true,
		"tenantId":   matched.TenantID,
		"tenantName": ten.Name,
		"accountId":  matched.ID,
		"label":      matched.Label,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.clearCookie(w, auth.CookieTech)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := s.requireTech(w, r)
	if !ok {
		return
	}
	ten, err := s.db.GetTenant(claims.TenantID)
	if err != nil {
		http.Error(w, "tenant missing", http.StatusInternalServerError)
		return
	}
	acc, _ := s.db.GetAccessAccount(claims.AccountID)
	writeJSON(w, map[string]any{
		"role":       "tech",
		"accountId":  claims.AccountID,
		"tenantId":   claims.TenantID,
		"tenantName": ten.Name,
		"label":      acc.Label,
	})
}
