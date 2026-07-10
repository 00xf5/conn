package agent

import (
	"time"

	"connect/internal/captureenc"
)

// StreamProfile holds all video/stream tunables. Defaults are the stable baseline;
// override via config.json or CLI flags — not by editing encoder internals.
type StreamProfile struct {
	Width        int
	Height       int
	FPS          int
	BitrateK     int
	GOP          int
	KeyIntMin    int
	WarmPrime    time.Duration
	StallTimeout time.Duration
	BitrateMin   int
	BitrateMax   int
}

// DefaultStreamProfile is the product baseline (1280×720 @ 20fps, 4.5 Mbps).
// Sharp enough for readable text; full 1080p can miss IDRs on some QSV hosts.
func DefaultStreamProfile() StreamProfile {
	return StreamProfile{
		Width:        1280,
		Height:       720,
		FPS:          20,
		BitrateK:     4500,
		GOP:          40,
		KeyIntMin:    20,
		WarmPrime:    1200 * time.Millisecond,
		StallTimeout: 15 * time.Second,
		BitrateMin:   1200,
		BitrateMax:   15000,
	}
}

func (p StreamProfile) FrameDuration() time.Duration {
	if p.FPS <= 0 {
		return time.Second / 20
	}
	return time.Second / time.Duration(p.FPS)
}

func (p StreamProfile) ClampBitrate(kbps int) int {
	if kbps < p.BitrateMin {
		return p.BitrateMin
	}
	if kbps > p.BitrateMax {
		return p.BitrateMax
	}
	return kbps
}

// ProfileFromConfig returns the effective stream profile for cfg (defaults applied).
func ProfileFromConfig(cfg Config) StreamProfile {
	p := DefaultStreamProfile()
	if cfg.Width > 0 {
		p.Width = cfg.Width
	}
	if cfg.Height > 0 {
		p.Height = cfg.Height
	}
	if cfg.FPS > 0 {
		p.FPS = cfg.FPS
	}
	if cfg.BitrateK > 0 {
		p.BitrateK = cfg.BitrateK
	}
	if cfg.GOP > 0 {
		p.GOP = cfg.GOP
	}
	if cfg.KeyIntMin > 0 {
		p.KeyIntMin = cfg.KeyIntMin
	}
	p.Width, p.Height = captureenc.AlignEncodeDimensions(p.Width, p.Height)
	return p
}

func alignStreamDimensions(w, h int) (int, int) {
	return captureenc.AlignEncodeDimensions(w, h)
}

// NormalizeConfig fills zero fields from DefaultStreamProfile.
// Soft-upgrades legacy ≤480p baselines so existing hosts get readable text
// without wiping intentional higher custom settings.
func NormalizeConfig(cfg Config) Config {
	p := DefaultStreamProfile()
	if isLegacyLowResStream(cfg) {
		cfg.Width = 0
		cfg.Height = 0
		if cfg.BitrateK > 0 && cfg.BitrateK <= 3500 {
			cfg.BitrateK = 0
		}
	}
	if cfg.FPS <= 0 {
		cfg.FPS = p.FPS
	}
	if cfg.BitrateK <= 0 {
		cfg.BitrateK = p.BitrateK
	}
	if cfg.Width <= 0 {
		cfg.Width = p.Width
	}
	if cfg.Height <= 0 {
		cfg.Height = p.Height
	}
	if cfg.GOP <= 0 {
		cfg.GOP = p.GOP
	}
	if cfg.KeyIntMin <= 0 {
		cfg.KeyIntMin = p.KeyIntMin
	}
	cfg.Width, cfg.Height = alignStreamDimensions(cfg.Width, cfg.Height)
	return cfg
}

func isLegacyLowResStream(cfg Config) bool {
	h := cfg.Height
	w := cfg.Width
	if h == 0 && w == 0 {
		return false
	}
	// Classic product baselines and near-misses (aligned 864×480, etc.).
	if h > 0 && h <= 480 {
		return true
	}
	if w > 0 && w <= 864 && h > 0 && h <= 486 {
		return true
	}
	return false
}
