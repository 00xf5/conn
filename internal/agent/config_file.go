package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// fileConfig matches deploy/config.json (camelCase keys).
type fileConfig struct {
	ServerURL   string `json:"serverUrl"`
	DeviceID    string `json:"deviceId"`
	TenantID    string `json:"tenantId"`
	Hostname    string `json:"hostname"`
	Monitor     int    `json:"monitor"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	FPS         int    `json:"fps"`
	Bitrate     int    `json:"bitrate"`
	GOP         int    `json:"gop"`
	KeyIntMin   int    `json:"keyIntMin"`
	InsecureTLS *bool  `json:"insecureTls"`
}

// LoadConfigFile reads the first existing config.json from standard locations.
func LoadConfigFile() (Config, bool) {
	for _, path := range configSearchPaths() {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
		var fc fileConfig
		if err := json.Unmarshal(b, &fc); err != nil {
			continue
		}
		cfg := Config{
			ServerURL: fc.ServerURL,
			DeviceID:  fc.DeviceID,
			TenantID:  fc.TenantID,
			Hostname:  fc.Hostname,
			Monitor:   fc.Monitor,
			Width:     fc.Width,
			Height:    fc.Height,
			FPS:       fc.FPS,
			BitrateK:  fc.Bitrate,
			GOP:       fc.GOP,
			KeyIntMin: fc.KeyIntMin,
		}
		if fc.InsecureTLS != nil {
			cfg.InsecureTLS = *fc.InsecureTLS
		}
		return NormalizeConfig(cfg), true
	}
	return Config{}, false
}

func configSearchPaths() []string {
	var paths []string
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		paths = append(paths, filepath.Join(local, "Connect", "config.json"))
	}
	paths = append(paths, "config.json")
	return paths
}

// MergeConfig applies CLI values over file config. Zero CLI values keep file values.
func MergeConfig(base Config, overrides Config) Config {
	out := base
	if overrides.ServerURL != "" {
		out.ServerURL = overrides.ServerURL
	}
	if overrides.DeviceID != "" {
		out.DeviceID = overrides.DeviceID
	}
	if overrides.TenantID != "" {
		out.TenantID = overrides.TenantID
	}
	if overrides.Hostname != "" {
		out.Hostname = overrides.Hostname
	}
	if overrides.Monitor > 0 {
		out.Monitor = overrides.Monitor
	}
	if overrides.Width > 0 {
		out.Width = overrides.Width
	}
	if overrides.Height > 0 {
		out.Height = overrides.Height
	}
	if overrides.FPS > 0 {
		out.FPS = overrides.FPS
	}
	if overrides.BitrateK > 0 {
		out.BitrateK = overrides.BitrateK
	}
	if overrides.GOP > 0 {
		out.GOP = overrides.GOP
	}
	if overrides.KeyIntMin > 0 {
		out.KeyIntMin = overrides.KeyIntMin
	}
	if overrides.InsecureTLS {
		out.InsecureTLS = true
	}
	return NormalizeConfig(out)
}
