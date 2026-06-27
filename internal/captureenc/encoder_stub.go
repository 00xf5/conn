//go:build !windows || !cgo || !experimental

package captureenc

import "fmt"

type Encoder struct{}

type Config struct {
	Monitor  int
	Width    int
	Height   int
	FPS      int
	BitrateK int
}

type Frame struct {
	Data      []byte
	KeyFrame  bool
	Timestamp uint64
}

func New(cfg Config) (*Encoder, error) {
	return &Encoder{}, nil
}

func (e *Encoder) EncoderName() string { return "stub-no-cgo" }

func (e *Encoder) CaptureSize() (int, int) { return 0, 0 }

func (e *Encoder) ReadFrame() (Frame, error) {
	return Frame{}, fmt.Errorf("captureenc requires CGO on Windows")
}

func (e *Encoder) SetBitrate(kbps int) error { return nil }

func (e *Encoder) Close() error { return nil }
