package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin = "admin"
	RoleTech  = "tech"

	CookieAdmin = "connect_admin"
	CookieTech  = "connect_tech"

	AccessCodeLen     = 20
	EnrollmentCodeLen = 16
	HostKeyLen        = 16
	accessAlphabet    = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

type TokenClaims struct {
	Role      string `json:"role"`
	AccountID string `json:"accountId,omitempty"`
	TenantID  string `json:"tenantId,omitempty"`
	Exp       int64  `json:"exp"`
}

type TokenSigner struct {
	secret []byte
}

func LoadOrCreateSecret(path string) (*TokenSigner, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return &TokenSigner{secret: data}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	return &TokenSigner{secret: secret}, nil
}

func (t *TokenSigner) Issue(claims TokenClaims, ttl time.Duration) (string, error) {
	if ttl == 0 {
		ttl = 12 * time.Hour
	}
	claims.Exp = time.Now().Add(ttl).Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, t.secret)
	_, _ = mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

func (t *TokenSigner) Parse(token string) (TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return TokenClaims{}, fmt.Errorf("invalid token")
	}
	mac := hmac.New(sha256.New, t.secret)
	_, _ = mac.Write([]byte(parts[0]))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(want, got) {
		return TokenClaims{}, fmt.Errorf("invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return TokenClaims{}, fmt.Errorf("invalid payload")
	}
	var c TokenClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return TokenClaims{}, err
	}
	if time.Now().Unix() > c.Exp {
		return TokenClaims{}, fmt.Errorf("expired")
	}
	return c, nil
}

func GenerateAccessCode() (string, error) {
	return formatGroupedCode(AccessCodeLen)
}

// GenerateEnrollmentCode returns a one-time host enrollment code (ENR-…).
func GenerateEnrollmentCode() (string, error) {
	s, err := formatGroupedCode(EnrollmentCodeLen)
	if err != nil {
		return "", err
	}
	return "ENR-" + s, nil
}

// GenerateHostKey returns a permanent Host GUI unlock key (HOST-…).
func GenerateHostKey() (string, error) {
	s, err := formatGroupedCode(HostKeyLen)
	if err != nil {
		return "", err
	}
	return "HOST-" + s, nil
}

func formatGroupedCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = accessAlphabet[int(b[i])%len(accessAlphabet)]
	}
	s := string(out)
	var parts []string
	for i := 0; i < len(s); i += 4 {
		end := i + 4
		if end > len(s) {
			end = len(s)
		}
		parts = append(parts, s[i:end])
	}
	return strings.Join(parts, "-"), nil
}

func NormalizeAccessCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	code = strings.TrimPrefix(code, "ENR")
	return code
}

func NormalizeEnrollmentCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	if strings.HasPrefix(code, "ENR-") {
		code = strings.TrimPrefix(code, "ENR-")
	} else if strings.HasPrefix(code, "ENR") {
		code = strings.TrimPrefix(code, "ENR")
	}
	code = strings.ReplaceAll(code, "-", "")
	return code
}

func NormalizeHostKey(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	if strings.HasPrefix(code, "HOST-") {
		code = strings.TrimPrefix(code, "HOST-")
	} else if strings.HasPrefix(code, "HOST") {
		code = strings.TrimPrefix(code, "HOST")
	}
	code = strings.ReplaceAll(code, "-", "")
	return code
}

func HashAccessCode(code string) (string, error) {
	norm := NormalizeAccessCode(code)
	if len(norm) < 12 {
		return "", fmt.Errorf("code too short")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(norm), bcrypt.DefaultCost)
	return string(h), err
}

func HashEnrollmentCode(code string) (string, error) {
	norm := NormalizeEnrollmentCode(code)
	if len(norm) < 12 {
		return "", fmt.Errorf("enrollment code too short")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(norm), bcrypt.DefaultCost)
	return string(h), err
}

func HashHostKey(code string) (string, error) {
	norm := NormalizeHostKey(code)
	if len(norm) < 12 {
		return "", fmt.Errorf("host key too short")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(norm), bcrypt.DefaultCost)
	return string(h), err
}

func CheckAccessCode(hash, code string) bool {
	norm := NormalizeAccessCode(code)
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(norm)) == nil
}

func CheckEnrollmentCode(hash, code string) bool {
	norm := NormalizeEnrollmentCode(code)
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(norm)) == nil
}

func CheckHostKey(hash, code string) bool {
	norm := NormalizeHostKey(code)
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(norm)) == nil
}
