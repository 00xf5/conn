//go:build windows && cgo

package agent

import (
	"fmt"
	"log"
	"time"

	"connect/internal/captureenc"
)

const (
	hostEmptyPollInterval = 2 * time.Millisecond
	hostRecoverAfter      = 60 // consecutive empty polls before native recover
)

// hostPipelineEncoder wraps the canonical captureenc.HostPipeline as a videoEncoder.
type hostPipelineEncoder struct {
	pipe       *captureenc.HostPipeline
	fps        int
	emptyPolls int
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
	return &hostPipelineEncoder{pipe: pipe, fps: prof.FPS}, nil
}

func (h *hostPipelineEncoder) ReadFrame() (videoFrame, error) {
	if h.pipe == nil {
		return videoFrame{}, fmt.Errorf("encoder closed")
	}

	deadline := time.Now().Add(hostReadBudget())
	for {
		au, err := h.pipe.ReadAccessUnit()
		if err != nil {
			return videoFrame{}, err
		}
		if len(au.Data) > 0 {
			h.emptyPolls = 0
			return videoFrame{
				Data:      au.Data,
				KeyFrame:  au.KeyFrame,
				Timestamp: au.Timestamp,
			}, nil
		}

		h.emptyPolls++
		if h.emptyPolls >= hostRecoverAfter {
			h.recoverPipeline()
		}

		if time.Now().After(deadline) {
			return videoFrame{}, fmt.Errorf("host pipeline: no valid frame within %s", hostReadBudget())
		}
		time.Sleep(hostEmptyPollInterval)
	}
}

func (h *hostPipelineEncoder) recoverPipeline() {
	if h.pipe == nil {
		return
	}
	log.Printf("agent: host pipeline recover (empty polls=%d)", h.emptyPolls)
	_ = h.pipe.RequestKeyframe()
	if err := h.pipe.Recover(); err != nil {
		log.Printf("agent: host pipeline recover failed: %v", err)
	} else {
		log.Printf("agent: host pipeline recovered")
	}
	h.emptyPolls = 0
}

func hostReadBudget() time.Duration {
	// One frame period plus slack for QSV priming / DXGI timeout.
	return 150 * time.Millisecond
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
