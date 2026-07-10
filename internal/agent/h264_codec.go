package agent

import (
	"fmt"
	"strings"
	"time"

	"connect/internal/captureenc"

	"github.com/pion/webrtc/v4"
)

// h264CodecCapabilityFromAnnexB builds WebRTC H.264 fmtp from the stream SPS.
// A fixed profile-level-id that does not match the bitstream causes blank video in Chrome.
func h264CodecCapabilityFromAnnexB(annexB []byte) webrtc.RTPCodecCapability {
	plid := "42e01f"
	if id := spsProfileLevelID(annexB); id != "" {
		plid = id
	}
	return webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: fmt.Sprintf("level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=%s", plid),
	}
}

func spsProfileLevelID(annexB []byte) string {
	for i := 0; i+5 < len(annexB); i++ {
		if annexB[i] != 0 || annexB[i+1] != 0 {
			continue
		}
		hdr := 3
		if annexB[i+2] == 0 && i+4 < len(annexB) && annexB[i+3] == 1 {
			hdr = 4
		} else if annexB[i+2] != 1 {
			continue
		}
		off := i + hdr
		if off >= len(annexB) {
			continue
		}
		if annexB[off]&0x1f != 7 {
			continue
		}
		if off+3 >= len(annexB) {
			return ""
		}
		return fmt.Sprintf("%02x%02x%02x", annexB[off+1], annexB[off+2], annexB[off+3])
	}
	return ""
}

func keyframeWaitTimeout(prof StreamProfile) time.Duration {
	// Worst-case natural IDR spacing is ~GOP frames at target fps; allow slack for
	// idle DXGI (no dirty frames) and slow HW encode startup.
	if prof.FPS <= 0 {
		return 10 * time.Second
	}
	gop := prof.GOP
	if gop <= 0 {
		gop = prof.FPS * 2
	}
	wait := time.Duration(gop*2) * time.Second / time.Duration(prof.FPS)
	if wait < 10*time.Second {
		wait = 10 * time.Second
	}
	if wait > 16*time.Second {
		wait = 16 * time.Second
	}
	return wait
}

func readLiveKeyframe(enc videoEncoder, minBytes int, timeout time.Duration) (videoFrame, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	lastReq := time.Time{}
	for time.Now().Before(deadline) {
		if lastReq.IsZero() || time.Since(lastReq) >= 400*time.Millisecond {
			requestEncoderKeyframe(enc)
			lastReq = time.Now()
		}
		f, err := enc.ReadFrame()
		if err != nil {
			if strings.Contains(err.Error(), "no valid frame within") && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return videoFrame{}, err
		}
		if f.KeyFrame && len(f.Data) >= minBytes && captureenc.ContainsNALType(f.Data, 5) {
			return f, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return videoFrame{}, fmt.Errorf("no live keyframe within timeout")
}
