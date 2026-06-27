package agent

import (
	"log"
	"os"
)

// videoFrame is one encoded video unit for WebRTC.
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

// openVideoEncoder opens the default gdigrab + ffmpeg pipeline on Windows.
// Set CONNECT_ENCODER_DXGI=1 for DXGI capture + live codec probe instead.
func openVideoEncoder(cfg Config) (videoEncoder, error) {
	codec := sessionCodec(cfg)
	if os.Getenv("CONNECT_ENCODER_DXGI") == "1" {
		codec = resolveEncoderCodec(cfg)
		enc, err := newDXGIFFmpegEncoder(cfg, codec)
		if err != nil {
			log.Printf("agent: dxgi encoder failed (%v); falling back to gdigrab", err)
		} else {
			log.Printf("agent: pipeline %s", enc.Name())
			return enc, nil
		}
	}
	enc, err := newGdiGrabFFmpegEncoder(cfg, codec)
	if err != nil {
		return nil, err
	}
	log.Printf("agent: pipeline %s", enc.Name())
	return enc, nil
}

// sessionCodec picks ffmpeg encoder without opening DXGI (for gdigrab path).
func sessionCodec(cfg Config) EncoderCodec {
	if v := os.Getenv("CONNECT_ENCODER_CODEC"); v != "" {
		c := EncoderCodec(v)
		for _, ok := range probeOrder {
			if ok == c {
				return c
			}
		}
	}
	if c, ok := loadCachedEncoderCodec(); ok {
		return c
	}
	return CodecQSV
}
