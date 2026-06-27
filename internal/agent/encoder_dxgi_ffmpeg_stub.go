//go:build !windows || !cgo

package agent

import "fmt"

func newDXGIFFmpegEncoder(cfg Config, codec EncoderCodec) (*dxgiFFmpegEncoder, error) {
	_ = cfg
	_ = codec
	return nil, fmt.Errorf("dxgi encoder requires windows with CGO")
}

type dxgiFFmpegEncoder struct{}

func (d *dxgiFFmpegEncoder) CaptureSize() (int, int)       { return 0, 0 }
func (d *dxgiFFmpegEncoder) Name() string                  { return "dxgi-unavailable" }
func (d *dxgiFFmpegEncoder) ReadFrame() (videoFrame, error) {
	return videoFrame{}, fmt.Errorf("encoder unavailable")
}
func (d *dxgiFFmpegEncoder) SetBitrate(kbps int) error { _ = kbps; return nil }
func (d *dxgiFFmpegEncoder) Close() error                { return nil }
