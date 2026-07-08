package agent

import (
	"fmt"
	"log"
	"os"
)

// videoFrame is one encoded video unit for any transport (WebRTC, native viewer, relay).
type videoFrame struct {
	Data      []byte
	KeyFrame  bool
	Timestamp uint64
}

type videoEncoder interface {
	ReadFrame() (videoFrame, error)
	SetBitrate(kbps int) error
	Close() error
	Name() string
	CaptureSize() (width, height int)
}

func requestEncoderKeyframe(enc videoEncoder) {
	type keyframer interface {
		RequestKeyframe() error
	}
	if kf, ok := enc.(keyframer); ok {
		if err := kf.RequestKeyframe(); err != nil {
			log.Printf("agent: request keyframe: %v", err)
		}
	}
}

// openVideoEncoder prefers in-process DXGI + hardware H.264 (QSV/NVENC/MF).
// Falls back to DXGI/gdigrab + ffmpeg libx264 when native init fails.
// CONNECT_ENCODER_FFMPEG=1 forces the ffmpeg subprocess path.
func openVideoEncoder(cfg Config) (videoEncoder, error) {
	if os.Getenv("CONNECT_ENCODER_FFMPEG") != "1" {
		if enc, err := openHostPipelineEncoder(cfg); err == nil {
			log.Printf("agent: pipeline capture=dxgi codec=%s", enc.Name())
			return enc, nil
		} else {
			log.Printf("agent: native encode unavailable (%v); trying ffmpeg", err)
		}
	}

	codec := CodecX264
	forceGdi := os.Getenv("CONNECT_ENCODER_GDIGRAB") == "1"
	if !forceGdi {
		enc, err := newDXGIFFmpegEncoder(cfg, codec)
		if err == nil {
			log.Printf("agent: pipeline capture=dxgi codec=%s", codec)
			return enc, nil
		}
		log.Printf("agent: dxgi ffmpeg failed (%v); trying gdigrab", err)
	}

	enc, err := newGdiGrabFFmpegEncoder(cfg, codec)
	if err != nil {
		return nil, fmt.Errorf("video encoder: %w", err)
	}
	log.Printf("agent: pipeline capture=gdigrab codec=%s", codec)
	return enc, nil
}
