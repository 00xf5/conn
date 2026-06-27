//go:build windows && cgo

package captureenc

/*
#cgo CFLAGS: -I${SRCDIR}/native
#cgo LDFLAGS: -ld3d11 -ldxgi -luuid -lole32

#include "capture_only.c"
#include "dxgi_capture.c"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type CaptureOnly struct {
	handle C.CaptureOnlyHandle
}

type CaptureConfig struct {
	Monitor int
	Width   int
	Height  int
	FPS     int
}

type NV12Frame struct {
	Data   []byte
	Pitch  int
	Width  int
	Height int
}

func NewCaptureOnly(cfg CaptureConfig) (*CaptureOnly, error) {
	if cfg.Monitor < 0 {
		cfg.Monitor = 0
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 20
	}
	cCfg := C.CaptureOnlyConfig{
		monitor_index: C.int(cfg.Monitor),
		width:         C.int(cfg.Width),
		height:        C.int(cfg.Height),
		fps:           C.int(cfg.FPS),
	}
	var handle C.CaptureOnlyHandle
	if rc := C.capture_only_init(&cCfg, &handle); rc != 0 {
		return nil, fmt.Errorf("capture_only_init failed: %d", rc)
	}
	return &CaptureOnly{handle: handle}, nil
}

func (c *CaptureOnly) ReadNV12() (NV12Frame, error) {
	if c == nil || c.handle == nil {
		return NV12Frame{}, fmt.Errorf("capture closed")
	}
	var fr C.CaptureOnlyFrame
	rc := C.capture_only_read(c.handle, &fr)
	if rc == 1 {
		return NV12Frame{}, nil
	}
	if rc != 0 {
		return NV12Frame{}, fmt.Errorf("capture_only_read: %d", rc)
	}
	defer C.capture_only_release(c.handle, &fr)
	if fr.size <= 0 || fr.data == nil {
		return NV12Frame{}, nil
	}
	data := C.GoBytes(unsafe.Pointer(fr.data), C.int(fr.size))
	return NV12Frame{
		Data:   data,
		Pitch:  int(fr.pitch),
		Width:  int(fr.width),
		Height: int(fr.height),
	}, nil
}

func (c *CaptureOnly) Close() error {
	if c == nil || c.handle == nil {
		return nil
	}
	C.capture_only_shutdown(c.handle)
	c.handle = nil
	return nil
}
