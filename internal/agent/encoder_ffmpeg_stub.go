//go:build !windows

package agent

import "fmt"

func newFFmpegEncoder(cfg Config) (videoEncoder, error) {
	_ = cfg
	return nil, fmt.Errorf("ffmpeg fallback is windows-only")
}
