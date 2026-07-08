package agent

import "testing"

func TestSPSProfileLevelID(t *testing.T) {
	// Annex-B SPS NAL (type 7): profile 66 (0x42), constraints 0xe0, level 31 (3.1)
	sps := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xe0, 0x1f, 0xab, 0x40}
	got := spsProfileLevelID(sps)
	if got != "42e01f" {
		t.Fatalf("profile-level-id=%q want 42e01f", got)
	}
}

func TestH264CodecCapabilityFromAnnexB(t *testing.T) {
	sps := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xe0, 0x1f}
	cap := h264CodecCapabilityFromAnnexB(sps)
	if cap.SDPFmtpLine != "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f" {
		t.Fatalf("fmtp=%q", cap.SDPFmtpLine)
	}
}
