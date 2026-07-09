package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFileStripsBOM(t *testing.T) {
	dir := t.TempDir()
	connectDir := filepath.Join(dir, "Connect")
	if err := os.MkdirAll(connectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(connectDir, "config.json")
	content := "\ufeff{\"serverUrl\":\"wss://example.test/ws\",\"width\":854,\"height\":480,\"fps\":20,\"bitrate\":2000}"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := os.Getenv("LOCALAPPDATA")
	t.Setenv("LOCALAPPDATA", dir)

	cfg, ok := LoadConfigFile()
	if !ok {
		t.Fatal("expected config to load")
	}
	if cfg.ServerURL != "wss://example.test/ws" {
		t.Fatalf("server URL = %q, want wss://example.test/ws", cfg.ServerURL)
	}
	_ = orig
}
