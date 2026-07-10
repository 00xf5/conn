package server

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"connect/internal/auth"
	"connect/internal/store"
)

func loadAdminToken(configured string) string {
	if configured != "" {
		return configured
	}
	if v := strings.TrimSpace(os.Getenv("CONNECT_ADMIN_TOKEN")); v != "" {
		return v
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	log.Printf("connectd: CONNECT_ADMIN_TOKEN not set — generated for this process: %s", tok)
	log.Printf("connectd: set CONNECT_ADMIN_TOKEN in the environment for a stable admin login")
	return tok
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.cfg.PublicURL, "https://"),
		MaxAge:   int(ttl.Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.cfg.PublicURL, "https://"),
		MaxAge:   -1,
	})
}

func (s *Server) cookieClaims(r *http.Request, name string) (auth.TokenClaims, bool) {
	c, err := r.Cookie(name)
	if err != nil || c.Value == "" || s.tokens == nil {
		return auth.TokenClaims{}, false
	}
	claims, err := s.tokens.Parse(c.Value)
	if err != nil {
		return auth.TokenClaims{}, false
	}
	return claims, true
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if claims, ok := s.cookieClaims(r, auth.CookieAdmin); ok && claims.Role == auth.RoleAdmin {
		return true
	}
	if tok := strings.TrimSpace(r.Header.Get("X-Admin-Token")); tok != "" && tok == s.adminToken {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (s *Server) requireTech(w http.ResponseWriter, r *http.Request) (auth.TokenClaims, bool) {
	claims, ok := s.cookieClaims(r, auth.CookieTech)
	if !ok || claims.Role != auth.RoleTech || claims.TenantID == "" || claims.AccountID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return auth.TokenClaims{}, false
	}
	if s.db != nil {
		acc, err := s.db.GetAccessAccount(claims.AccountID)
		if err != nil || acc.Status == store.StatusRevoked {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return auth.TokenClaims{}, false
		}
	}
	return claims, true
}
