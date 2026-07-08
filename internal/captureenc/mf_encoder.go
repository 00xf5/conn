//go:build windows && cgo

package captureenc

/*
#include "native/mf_h264_encode.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// MFH264Encoder is an in-process Windows Media Foundation H.264 encoder.
type MFH264Encoder struct {
	enc  *C.MfH264Enc
	name string
}

// MFEncodedFrame is one H.264 access unit from the MF encoder.
type MFEncodedFrame struct {
	Data     []byte
	KeyFrame bool
}

func NewMFH264Encoder(width, height, fps, bitrateK int) (*MFH264Encoder, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions %dx%d", width, height)
	}
	if fps <= 0 {
		fps = 20
	}
	if bitrateK <= 0 {
		bitrateK = 2000
	}
	nameBuf := make([]byte, 64)
	var handle *C.MfH264Enc
	rc := C.mf_h264_enc_init(
		&handle,
		C.int(width), C.int(height), C.int(fps), C.int(bitrateK),
		(*C.char)(unsafe.Pointer(&nameBuf[0])), C.int(len(nameBuf)),
	)
	if rc != 0 {
		return nil, fmt.Errorf("mf_h264_enc_init: %d", rc)
	}
	name := cStringToGo(nameBuf)
	if name == "" {
		name = "mf-h264"
	}
	return &MFH264Encoder{enc: handle, name: name}, nil
}

func cStringToGo(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func (e *MFH264Encoder) Name() string {
	if e == nil {
		return ""
	}
	return e.name
}

func (e *MFH264Encoder) EncodeNV12(nv12 []byte, pitch, width, height int) (MFEncodedFrame, error) {
	if e == nil || e.enc == nil {
		return MFEncodedFrame{}, fmt.Errorf("encoder closed")
	}
	if len(nv12) == 0 {
		return MFEncodedFrame{}, nil
	}
	var pkt C.MfH264Packet
	rc := C.mf_h264_enc_encode(
		e.enc,
		(*C.uint8_t)(unsafe.Pointer(&nv12[0])),
		C.int(pitch), C.int(width), C.int(height),
		&pkt,
	)
	if rc == 1 {
		return MFEncodedFrame{}, nil
	}
	if rc != 0 {
		return MFEncodedFrame{}, fmt.Errorf("mf_h264_enc_encode: %d", rc)
	}
	defer C.mf_h264_enc_release_packet(&pkt)
	if pkt.size <= 0 || pkt.data == nil {
		return MFEncodedFrame{}, nil
	}
	data := C.GoBytes(unsafe.Pointer(pkt.data), C.int(pkt.size))
	return MFEncodedFrame{
		Data:     data,
		KeyFrame: pkt.keyframe != 0,
	}, nil
}

func (e *MFH264Encoder) SetBitrate(kbps int) error {
	if e == nil || e.enc == nil {
		return fmt.Errorf("encoder closed")
	}
	if kbps <= 0 {
		return nil
	}
	if rc := C.mf_h264_enc_set_bitrate(e.enc, C.int(kbps)); rc != 0 {
		return fmt.Errorf("mf_h264_enc_set_bitrate: %d", rc)
	}
	return nil
}

func (e *MFH264Encoder) Close() error {
	if e == nil || e.enc == nil {
		return nil
	}
	C.mf_h264_enc_shutdown(e.enc)
	e.enc = nil
	return nil
}
