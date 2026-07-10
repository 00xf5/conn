package agent

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConfigPath returns the preferred writable config.json path (next to the exe).
func ConfigPath() string {
	return filepath.Join(DataDir(), "config.json")
}

// SaveConfigFile writes cfg next to the agent executable (no BOM).
func SaveConfigFile(cfg Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	fc := fileConfig{
		ServerURL: cfg.ServerURL,
		DeviceID:  cfg.DeviceID,
		TenantID:  cfg.TenantID,
		Hostname:  cfg.Hostname,
		Monitor:   cfg.Monitor,
		Width:     cfg.Width,
		Height:    cfg.Height,
		FPS:       cfg.FPS,
		Bitrate:   cfg.BitrateK,
		GOP:       cfg.GOP,
		KeyIntMin: cfg.KeyIntMin,
	}
	insecure := cfg.InsecureTLS
	fc.InsecureTLS = &insecure
	b, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// EnrollWithCode redeems a one-time enrollment code and returns tenantId.
func EnrollWithCode(serverURL, enrollmentCode, deviceID, hostname string, insecureTLS bool) (tenantID, tenantName string, err error) {
	base := strings.TrimSpace(serverURL)
	if base == "" {
		return "", "", fmt.Errorf("server URL required")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", "", err
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	case "http", "https":
	default:
		return "", "", fmt.Errorf("unsupported server URL scheme")
	}
	u.Path = "/api/agent/enroll"
	u.RawQuery = ""
	u.Fragment = ""

	payload, _ := json.Marshal(map[string]string{
		"enrollmentCode": enrollmentCode,
		"deviceId":       deviceID,
		"hostname":       hostname,
	})
	client := &http.Client{Timeout: 20 * time.Second}
	if insecureTLS {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // LAN self-signed connectd cert
		}
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", "", fmt.Errorf("enroll failed: %s", strings.TrimSpace(string(body)))
	}
	var out struct {
		TenantID   string `json:"tenantId"`
		TenantName string `json:"tenantName"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", err
	}
	if out.TenantID == "" {
		return "", "", fmt.Errorf("enroll response missing tenantId")
	}
	return out.TenantID, out.TenantName, nil
}
