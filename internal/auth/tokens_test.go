package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAccessCodeRoundTrip(t *testing.T) {
	code, err := GenerateAccessCode()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashAccessCode(code)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckAccessCode(hash, code) {
		t.Fatal("check failed raw")
	}
	if !CheckAccessCode(hash, NormalizeAccessCode(code)) {
		t.Fatal("check failed normalized")
	}
	if CheckAccessCode(hash, "WRONG-CODE-WRONG-CODE") {
		t.Fatal("false positive")
	}
}

func TestTokenSigner(t *testing.T) {
	signer, err := LoadOrCreateSecret(filepath.Join(t.TempDir(), "auth.secret"))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := signer.Issue(TokenClaims{Role: RoleTech, AccountID: "a1", TenantID: "t1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := signer.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.Role != RoleTech || c.AccountID != "a1" || c.TenantID != "t1" {
		t.Fatalf("%+v", c)
	}
	expired, err := signer.Issue(TokenClaims{Role: RoleAdmin}, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signer.Parse(expired); err == nil {
		t.Fatal("expected expiry error")
	}
}
