//go:build windows && cgo

package agent

import (
	"fmt"
	"log"
	"time"

	"connect/internal/captureenc"
)

// hostPipelineEncoder wraps the canonical captureenc.HostPipeline as a videoEncoder.
type hostPipelineEncoder struct {
	pipe *captureenc.HostPipeline
}

func openHostPipelineEncoder(cfg Config) (*hostPipelineEncoder, error) {
	prof := ProfileFromConfig(cfg)
	w, h := captureenc.AlignEncodeDimensions(prof.Width, prof.Height)
	pipe, err := captureenc.OpenHostPipeline(captureenc.HostConfig{
		Monitor:  cfg.Monitor,
		Width:    w,
		Height:   h,
		FPS:      prof.FPS,
		BitrateK: prof.BitrateK,
	})
	if err != nil {
		return nil, err
	}
	cw, ch := pipe.CaptureSize()
	log.Printf("agent: host pipeline dxgi %dx%d -> %s @ %dkbps", cw, ch, pipe.EncoderName(), prof.BitrateK)
	return &hostPipelineEncoder{pipe: pipe}, nil
}

func (h *hostPipelineEncoder) ReadFrame() (videoFrame, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		au, err := h.pipe.ReadAccessUnit()
		if err != nil {
			return videoFrame{}, err
		}
		if len(au.Data) == 0 {
			if time.Now().After(deadline) {
				return videoFrame{}, fmt.Errorf("host pipeline: no valid frame within 5s")
			}
			time.Sleep(1 * time.Millisecond)
			continue
		}
		return videoFrame{Data: au.Data, KeyFrame: au.KeyFrame}, nil
	}
}

func (h *hostPipelineEncoder) SetBitrate(kbps int) error {
	if h.pipe == nil {
		return fmt.Errorf("encoder closed")
	}
	return h.pipe.SetBitrate(kbps)
}

func (h *hostPipelineEncoder) RequestKeyframe() error {
	if h.pipe == nil {
		return fmt.Errorf("encoder closed")
	}
	return h.pipe.RequestKeyframe()
}

func (h *hostPipelineEncoder) Close() error {
	if h.pipe == nil {
		return nil
	}
	err := h.pipe.Close()
	h.pipe = nil
	return err
}

func (h *hostPipelineEncoder) Name() string {
	if h.pipe == nil {
		return "host-pipeline"
	}
	return "dxgi-" + h.pipe.EncoderName()
}

func (h *hostPipelineEncoder) CaptureSize() (int, int) {
	if h.pipe == nil {
		return 0, 0
	}
	return h.pipe.CaptureSize()
}
