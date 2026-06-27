package agent

import "time"

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

// DefaultStreamProfile is the frozen LAN baseline (854×480 @ 20fps, 2 Mbps).
func DefaultStreamProfile() StreamProfile {
	return StreamProfile{
		Width:        854,
		Height:       480,
		FPS:          20,
		BitrateK:     2000,
		GOP:          40,
		KeyIntMin:    20,
		WarmPrime:    1200 * time.Millisecond,
	StallTimeout: 15 * time.Second,
		BitrateMin:   800,
		BitrateMax:   12000,
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
	return p
}

// NormalizeConfig fills zero fields from DefaultStreamProfile.
func NormalizeConfig(cfg Config) Config {
	p := DefaultStreamProfile()
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
	return cfg
}
