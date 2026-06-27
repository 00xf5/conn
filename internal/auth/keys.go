package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

type KeyPair struct {
	Public  ed25519.PublicKey
	Private ed25519.PrivateKey
}

func GenerateKeyPair() (KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{Public: pub, Private: priv}, nil
}

func (k KeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(k.Public)
}

func LoadOrCreateKeyPair(path string) (KeyPair, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(data)
		return KeyPair{Public: priv.Public().(ed25519.PublicKey), Private: priv}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return KeyPair{}, err
	}
	kp, err := GenerateKeyPair()
	if err != nil {
		return KeyPair{}, err
	}
	if err := os.WriteFile(path, []byte(kp.Private), 0o600); err != nil {
		return KeyPair{}, err
	}
	return kp, nil
}

func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length")
	}
	return ed25519.PublicKey(raw), nil
}
