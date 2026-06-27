package server

import "testing"

func TestLoadICEConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("CONNECT_STUN_URLS", "")
	t.Setenv("CONNECT_TURN_URL", "")
	t.Setenv("CONNECT_TURN_SECRET", "")
	cfg := LoadICEConfigFromEnv()
	if len(cfg.DefaultSTUN) != 2 {
		t.Fatalf("expected 2 default STUN URLs, got %v", cfg.DefaultSTUN)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" stun:a:1 , stun:b:2 ")
	if len(got) != 2 || got[0] != "stun:a:1" || got[1] != "stun:b:2" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestBuildExternalTURN(t *testing.T) {
	cfg := ICEConfig{
		ExternalTURNURL:    "turn:example.com:3478?transport=udp",
		ExternalTURNSecret: "test-secret-value",
	}
	srv, ok := cfg.buildExternalTURN()
	if !ok {
		t.Fatal("expected external TURN")
	}
	if srv.URLs[0] != cfg.ExternalTURNURL {
		t.Fatalf("url mismatch: %v", srv.URLs)
	}
	if srv.Username == "" || srv.Credential == "" {
		t.Fatal("expected REST credentials")
	}
}
