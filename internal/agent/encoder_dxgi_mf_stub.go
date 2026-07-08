//go:build !windows || !cgo

package agent

import "fmt"

func newDXGIMFEncoder(cfg Config) (*dxgiMFEncoder, error) {
	_ = cfg
	return nil, fmt.Errorf("dxgi mf encoder requires windows with CGO")
}

type dxgiMFEncoder struct{}

func (d *dxgiMFEncoder) ReadFrame() (videoFrame, error) {
	return videoFrame{}, fmt.Errorf("encoder unavailable")
}
func (d *dxgiMFEncoder) SetBitrate(kbps int) error       { _ = kbps; return nil }
func (d *dxgiMFEncoder) Close() error                    { return nil }
func (d *dxgiMFEncoder) Name() string                    { return "dxgi-mf-unavailable" }
func (d *dxgiMFEncoder) CaptureSize() (int, int)           { return 0, 0 }
