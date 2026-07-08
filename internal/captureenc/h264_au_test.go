package captureenc

import "testing"

func TestValidateH264AccessUnit(t *testing.T) {
	idr := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x10}
	idr = append(idr, make([]byte, MinKeyframeBytes-len(idr))...)

	if err := ValidateH264AccessUnit(idr, true); err != nil {
		t.Fatalf("valid IDR: %v", err)
	}

	spsOnly := append([]byte{0x00, 0x00, 0x01, 0x67, 0x42, 0xe0, 0x1f}, make([]byte, MinKeyframeBytes)...)
	if err := ValidateH264AccessUnit(spsOnly, true); err == nil {
		t.Fatal("SPS-only must not pass as keyframe")
	}

	p := append([]byte{0x00, 0x00, 0x01, 0x41, 0x9a}, make([]byte, MinDeltaBytes-5)...)
	if err := ValidateH264AccessUnit(p, false); err != nil {
		t.Fatalf("valid P-frame: %v", err)
	}

	tiny := []byte{0x00, 0x00, 0x01, 0x41, 0x01}
	if err := ValidateH264AccessUnit(tiny, false); err == nil {
		t.Fatal("tiny P-frame must fail")
	}
}

func TestContainsNALType(t *testing.T) {
	b := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x00, 0x01, 0x65, 0x88}
	if !ContainsNALType(b, 7) || !ContainsNALType(b, 5) {
		t.Fatal("expected SPS and IDR")
	}
}
