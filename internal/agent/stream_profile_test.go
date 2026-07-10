package agent

import "testing"

func TestNormalizeConfigSoftUpgradesOldBaseline(t *testing.T) {
	cfg := NormalizeConfig(Config{Width: 854, Height: 480, BitrateK: 2000})
	if cfg.Width != 1280 || cfg.Height != 720 {
		t.Fatalf("resolution got %dx%d want 1280x720", cfg.Width, cfg.Height)
	}
	if cfg.BitrateK != 3500 {
		t.Fatalf("bitrate got %d want 3500", cfg.BitrateK)
	}
}

func TestNormalizeConfigKeepsCustomSettings(t *testing.T) {
	cfg := NormalizeConfig(Config{Width: 1920, Height: 1080, BitrateK: 6000, FPS: 30})
	if cfg.Width != 1920 || cfg.Height != 1080 || cfg.BitrateK != 6000 || cfg.FPS != 30 {
		t.Fatalf("custom settings mutated: %+v", cfg)
	}
}

func TestClampBitrate(t *testing.T) {
	p := DefaultStreamProfile()
	if got := p.ClampBitrate(100); got != p.BitrateMin {
		t.Fatalf("floor: got %d want %d", got, p.BitrateMin)
	}
	if got := p.ClampBitrate(99999); got != p.BitrateMax {
		t.Fatalf("ceil: got %d want %d", got, p.BitrateMax)
	}
}
