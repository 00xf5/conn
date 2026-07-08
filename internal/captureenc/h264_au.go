package captureenc

import (
	"fmt"
)

// H.264 access-unit limits shared by every consumer (WebRTC, native viewer, relay).
const (
	MinKeyframeBytes = 500
	MinDeltaBytes    = 200
)

// ValidateH264AccessUnit rejects truncated or structurally invalid access units.
func ValidateH264AccessUnit(data []byte, keyframe bool) error {
	if len(data) == 0 {
		return fmt.Errorf("empty access unit")
	}
	min := MinDeltaBytes
	if keyframe {
		min = MinKeyframeBytes
		if !ContainsNALType(data, 5) {
			return fmt.Errorf("keyframe missing IDR (type 5)")
		}
	}
	if len(data) < min {
		return fmt.Errorf("access unit too small (%d < %d)", len(data), min)
	}
	return nil
}

// ContainsNALType reports whether an Annex-B bitstream contains a NAL of the given type.
func ContainsNALType(data []byte, wantType byte) bool {
	for i := 0; i+4 < len(data); i++ {
		if data[i] != 0 || data[i+1] != 0 {
			continue
		}
		hdr := 3
		if data[i+2] == 0 && i+3 < len(data) && data[i+3] == 1 {
			hdr = 4
		} else if data[i+2] != 1 {
			continue
		}
		off := i + hdr
		if off < len(data) && data[off]&0x1f == wantType {
			return true
		}
	}
	return false
}
