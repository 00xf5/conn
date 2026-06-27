package agent

import (
	"encoding/json"

	"github.com/pion/webrtc/v4"
)

var h264Capability = webrtc.RTPCodecCapability{
	MimeType:    webrtc.MimeTypeH264,
	ClockRate:   90000,
	SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e028",
}

type Config struct {
	ServerURL   string
	DeviceID    string
	Hostname    string
	Monitor     int
	Width       int
	Height      int
	FPS         int
	BitrateK    int
	GOP         int
	KeyIntMin   int
	InsecureTLS bool
}

type signalingEnvelope struct {
	Type     string          `json:"type"`
	Session  string          `json:"session,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	DeviceID string          `json:"deviceId,omitempty"`
}
