//go:build windows && cgo

package captureenc

/*
#include "native/bridge.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// HostConfig is the canonical capture+encode profile (DXGI → NVENC/QSV in-process).
type HostConfig struct {
	Monitor  int
	Width    int
	Height   int
	FPS      int
	BitrateK int
}

// HostPipeline is the production host encode path: DXGI NV12 → hardware H.264 → Annex-B AU.
type HostPipeline struct {
	handle C.CaptureEncHandle
}

// AlignEncodeDimensions rounds to H.264 macroblock boundaries (16px).
func AlignEncodeDimensions(w, h int) (int, int) {
	if w <= 0 {
		w = 854
	}
	if h <= 0 {
		h = 480
	}
	w = (w + 15) & ^15
	if h&1 != 0 {
		h++
	}
	return w, h
}

// FitEncodeDimensions downscales for in-process HW encoders that cannot sustain
// full desktop resolution on low-power iGPUs (pixels scale ~linearly with encode cost).
func FitEncodeDimensions(w, h, maxW int) (int, int) {
	if maxW > 0 && w > maxW {
		h = h * maxW / w
		w = maxW
	}
	return AlignEncodeDimensions(w, h)
}

func OpenHostPipeline(cfg HostConfig) (*HostPipeline, error) {
	if cfg.Monitor < 0 {
		cfg.Monitor = 0
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 20
	}
	if cfg.BitrateK <= 0 {
		cfg.BitrateK = 2000
	}
	cfg.Width, cfg.Height = AlignEncodeDimensions(cfg.Width, cfg.Height)

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
		return nil, hostInitError(rc)
	}
	return &HostPipeline{handle: handle}, nil
}

func hostInitError(code C.int) error {
	switch code {
	case -10, -11, -12, -13, -14:
		return fmt.Errorf("host pipeline: DXGI init failed (%d)", code)
	case -2:
		return fmt.Errorf("host pipeline: Intel QSV DLL missing")
	case -3:
		return fmt.Errorf("host pipeline: QSV MFXInit failed")
	case -4:
		return fmt.Errorf("host pipeline: QSV D3D11 bind failed")
	case -5, -6:
		return fmt.Errorf("host pipeline: QSV encode init failed (%d)", code)
	case -60:
		return fmt.Errorf("host pipeline: no in-process encoder (QSV and MF both failed)")
	default:
		return fmt.Errorf("host pipeline init failed (%d)", code)
	}
}

func (p *HostPipeline) EncoderName() string {
	if p == nil || p.handle == nil {
		return "none"
	}
	return C.GoString(C.captureenc_encoder_name(p.handle))
}

func (p *HostPipeline) CaptureSize() (int, int) {
	if p == nil || p.handle == nil {
		return 0, 0
	}
	var w, h C.int
	C.captureenc_capture_size(p.handle, &w, &h)
	return int(w), int(h)
}

func (p *HostPipeline) ReadAccessUnit() (AccessUnit, error) {
	if p == nil || p.handle == nil {
		return AccessUnit{}, fmt.Errorf("host pipeline closed")
	}
	var cFrame C.CaptureEncFrame
	rc := C.captureenc_read_frame(p.handle, &cFrame)
	if rc == 1 {
		return AccessUnit{}, nil
	}
	if rc != 0 {
		return AccessUnit{}, fmt.Errorf("host pipeline read: %d", rc)
	}
	defer C.captureenc_release_frame(p.handle, &cFrame)

	size := int(cFrame.size)
	if size <= 0 || cFrame.data == nil {
		return AccessUnit{}, nil
	}
	data := C.GoBytes(unsafe.Pointer(cFrame.data), C.int(size))
	key := cFrame.keyframe != 0
	if err := ValidateH264AccessUnit(data, key); err != nil {
		// Drop corrupt units — never forward to transports.
		return AccessUnit{}, nil
	}
	return AccessUnit{
		Data:     data,
		KeyFrame: key,
		Timestamp: uint64(cFrame.timestamp_us),
	}, nil
}

func (p *HostPipeline) Recover() error {
	if p == nil || p.handle == nil {
		return fmt.Errorf("host pipeline closed")
	}
	if rc := C.captureenc_recover(p.handle); rc != 0 {
		return fmt.Errorf("host pipeline recover: %d", rc)
	}
	return nil
}

func (p *HostPipeline) SetBitrate(kbps int) error {
	if p == nil || p.handle == nil {
		return fmt.Errorf("host pipeline closed")
	}
	if kbps <= 0 {
		return nil
	}
	if rc := C.captureenc_set_bitrate(p.handle, C.int(kbps)); rc != 0 {
		return fmt.Errorf("host pipeline set bitrate: %d", rc)
	}
	return nil
}

func (p *HostPipeline) RequestKeyframe() error {
	if p == nil || p.handle == nil {
		return fmt.Errorf("host pipeline closed")
	}
	if rc := C.captureenc_request_keyframe(p.handle); rc != 0 {
		return fmt.Errorf("host pipeline request keyframe: %d", rc)
	}
	return nil
}

func (p *HostPipeline) Close() error {
	if p == nil || p.handle == nil {
		return nil
	}
	C.captureenc_shutdown(p.handle)
	p.handle = nil
	return nil
}

// AccessUnit is one validated H.264 access unit for any transport (WebRTC, file, relay).
type AccessUnit struct {
	Data      []byte
	KeyFrame  bool
	Timestamp uint64
}
