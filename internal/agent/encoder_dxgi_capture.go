//go:build windows && cgo

package agent

import (
	"fmt"
	"time"

	"connect/internal/captureenc"
)

type captureencOnly struct {
	cap *captureenc.CaptureOnly
}

func openDXGICapture(cfg Config) (*captureencOnly, int, int, error) {
	prof := ProfileFromConfig(cfg)
	w, h := prof.Width, prof.Height
	cap, err := captureenc.NewCaptureOnly(captureenc.CaptureConfig{
		Monitor: cfg.Monitor,
		Width:   w,
		Height:  h,
		FPS:     cfg.FPS,
	})
	if err != nil {
		return nil, 0, 0, err
	}
	for i := 0; i < 20; i++ {
		frame, err := cap.ReadNV12()
		if err != nil {
			_ = cap.Close()
			return nil, 0, 0, err
		}
		if len(frame.Data) > 0 && frame.Width > 0 && frame.Height > 0 {
			return &captureencOnly{cap: cap}, frame.Width, frame.Height, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = cap.Close()
	return nil, 0, 0, fmt.Errorf("dxgi capture produced no frames")
}

func (c *captureencOnly) ReadNV12() (captureenc.NV12Frame, error) {
	if c == nil || c.cap == nil {
		return captureenc.NV12Frame{}, fmt.Errorf("capture closed")
	}
	return c.cap.ReadNV12()
}

func (c *captureencOnly) Close() error {
	if c == nil || c.cap == nil {
		return nil
	}
	return c.cap.Close()
}
