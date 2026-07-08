package agent

import "connect/internal/captureenc"

// acceptVideoFrame returns true when an access unit is safe to send to WebRTC decoders.
func acceptVideoFrame(fr videoFrame) bool {
	if len(fr.Data) == 0 {
		return false
	}
	return captureenc.ValidateH264AccessUnit(fr.Data, fr.KeyFrame) == nil
}
