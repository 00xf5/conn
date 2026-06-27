//go:build windows && cgo && experimental

package captureenc

/*
#cgo CFLAGS: -I${SRCDIR}/native
#include "bridge.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Encoder wraps DXGI desktop duplication + hardware H.264 (NVENC or Intel QSV).
type Encoder struct {
	handle C.CaptureEncHandle
}

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

func initError(code C.int) error {
	switch code {
	case -10, -11, -12, -13, -14:
		return fmt.Errorf("captureenc_init failed: %d (DXGI desktop duplication setup failed)", code)
	case -20:
		return fmt.Errorf("captureenc_init failed: %d (capture reinit failed)", code)
	case -2:
		return fmt.Errorf("captureenc_init failed: Intel QSV DLL load failed (need libmfxhw64.dll from Intel graphics driver)")
	case -3:
		return fmt.Errorf("captureenc_init failed: Intel QSV MFXInit failed (no hardware session)")
	case -4:
		return fmt.Errorf("captureenc_init failed: Intel QSV D3D11 bind failed")
	case -5:
		return fmt.Errorf("captureenc_init failed: Intel QSV ENCODE_Query failed (bad video params)")
	case -6:
		return fmt.Errorf("captureenc_init failed: Intel QSV ENCODE_Init failed")
	case -7:
		return fmt.Errorf("captureenc_init failed: Intel QSV out of memory")
	case -21:
		return fmt.Errorf("captureenc_init failed: %d (no HW encoder)", code)
	default:
		return fmt.Errorf("captureenc_init failed: %d", code)
	}
}

func New(cfg Config) (*Encoder, error) {
	if cfg.Monitor < 0 {
		cfg.Monitor = 0
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.BitrateK <= 0 {
		cfg.BitrateK = 4000
	}
	cCfg := C.CaptureEncConfig{
		monitor_index: C.int(cfg.Monitor),
		width:         C.int(cfg.Width),
		height:        C.int(cfg.Height),
		fps:           C.int(cfg.FPS),
		bitrate_kbps:  C.int(cfg.BitrateK),
	}
	var handle C.CaptureEncHandle
	rc := C.captureenc_init(&cCfg, &handle)
	if rc != 0 {
		return nil, initError(rc)
	}
	return &Encoder{handle: handle}, nil
}

func (e *Encoder) EncoderName() string {
	if e == nil || e.handle == nil {
		return "none"
	}
	return C.GoString(C.captureenc_encoder_name(e.handle))
}

func (e *Encoder) CaptureSize() (int, int) {
	if e == nil || e.handle == nil {
		return 0, 0
	}
	var w, h C.int
	C.captureenc_capture_size(e.handle, &w, &h)
	return int(w), int(h)
}

func (e *Encoder) ReadFrame() (Frame, error) {
	if e == nil || e.handle == nil {
		return Frame{}, fmt.Errorf("encoder closed")
	}
	var cFrame C.CaptureEncFrame
	rc := C.captureenc_read_frame(e.handle, &cFrame)
	if rc == 1 {
		return Frame{}, nil // no frame yet
	}
	if rc != 0 {
		return Frame{}, fmt.Errorf("captureenc_read_frame: %d", rc)
	}
	defer C.captureenc_release_frame(e.handle, &cFrame)

	size := int(cFrame.size)
	if size <= 0 || cFrame.data == nil {
		return Frame{}, nil
	}
	data := C.GoBytes(unsafe.Pointer(cFrame.data), C.int(size))
	return Frame{
		Data:      data,
		KeyFrame:  cFrame.keyframe != 0,
		Timestamp: uint64(cFrame.timestamp_us),
	}, nil
}

func (e *Encoder) SetBitrate(kbps int) error {
	if e == nil || e.handle == nil {
		return fmt.Errorf("encoder closed")
	}
	rc := C.captureenc_set_bitrate(e.handle, C.int(kbps))
	if rc != 0 {
		return fmt.Errorf("captureenc_set_bitrate: %d", rc)
	}
	return nil
}

func (e *Encoder) Close() error {
	if e == nil || e.handle == nil {
		return nil
	}
	C.captureenc_shutdown(e.handle)
	e.handle = nil
	return nil
}
