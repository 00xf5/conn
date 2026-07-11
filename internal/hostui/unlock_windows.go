//go:build windows

package hostui

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connect/internal/agent"
	"connect/internal/auth"

	"golang.org/x/crypto/bcrypt"
)

type unlockFile struct {
	DeviceID   string `json:"deviceId"`
	KeyHash    string `json:"keyHash"`
	UnlockedAt string `json:"unlockedAt"`
}

func unlockFilePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Connect", "host-unlock.json")
}

func loadUnlockFile() (unlockFile, bool) {
	b, err := os.ReadFile(unlockFilePath())
	if err != nil {
		return unlockFile{}, false
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var u unlockFile
	if json.Unmarshal(b, &u) != nil || strings.TrimSpace(u.DeviceID) == "" || u.KeyHash == "" {
		return unlockFile{}, false
	}
	return u, true
}

func saveUnlockFile(deviceID, hostKey string) error {
	hash, err := auth.HashHostKey(hostKey)
	if err != nil {
		return err
	}
	u := unlockFile{
		DeviceID:   strings.TrimSpace(deviceID),
		KeyHash:    hash,
		UnlockedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	path := unlockFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func clearUnlockFile() {
	_ = os.Remove(unlockFilePath())
}

func rememberedUnlocked(deviceID string) bool {
	u, ok := loadUnlockFile()
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(u.DeviceID), strings.TrimSpace(deviceID))
}

func verifyRememberedKey(hostKey string) bool {
	u, ok := loadUnlockFile()
	if !ok {
		return false
	}
	norm := auth.NormalizeHostKey(hostKey)
	return bcrypt.CompareHashAndPassword([]byte(u.KeyHash), []byte(norm)) == nil
}

type hostIdentity struct {
	DeviceID    string
	Hostname    string
	ServerURL   string
	InsecureTLS bool
	Enrolled    bool
}

func loadHostIdentity() hostIdentity {
	cfg, ok := agent.LoadConfigFile()
	if !ok {
		host, _ := os.Hostname()
		return hostIdentity{Hostname: host}
	}
	host := cfg.Hostname
	if host == "" {
		host, _ = os.Hostname()
	}
	return hostIdentity{
		DeviceID:    cfg.DeviceID,
		Hostname:    host,
		ServerURL:   cfg.ServerURL,
		InsecureTLS: cfg.InsecureTLS,
		Enrolled:    strings.TrimSpace(cfg.TenantID) != "" && strings.TrimSpace(cfg.DeviceID) != "",
	}
}

func httpBaseFromWS(serverWS string) string {
	u := strings.TrimSpace(serverWS)
	u = strings.Replace(u, "wss://", "https://", 1)
	u = strings.Replace(u, "ws://", "http://", 1)
	u = strings.TrimSuffix(u, "/ws")
	u = strings.TrimRight(u, "/")
	return u
}

func verifyHostKeyOnline(id hostIdentity, hostKey string) error {
	if !id.Enrolled {
		return fmt.Errorf("this PC is not enrolled yet — run WorthyJoin Setup first")
	}
	base := httpBaseFromWS(id.ServerURL)
	if base == "" {
		return fmt.Errorf("server URL missing from config")
	}
	body, _ := json.Marshal(map[string]string{
		"deviceId": id.DeviceID,
		"hostKey":  hostKey,
	})
	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: id.InsecureTLS}, //nolint:gosec
		},
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/agent/host-key/verify", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WorthyJoin-Host")
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach server — check network: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = res.Status
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func unlockWithKey(hostKey string) error {
	hostKey = strings.TrimSpace(hostKey)
	if hostKey == "" {
		return fmt.Errorf("enter the host key from your tech")
	}
	id := loadHostIdentity()
	if rememberedUnlocked(id.DeviceID) && verifyRememberedKey(hostKey) {
		return nil
	}
	if err := verifyHostKeyOnline(id, hostKey); err != nil {
		return err
	}
	return saveUnlockFile(id.DeviceID, hostKey)
}
