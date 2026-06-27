package agent

import "fmt"

// EncoderCodec is the ffmpeg video encoder name (frozen set — do not add without bumping cacheVersion).
type EncoderCodec string

const (
	CodecNVENC EncoderCodec = "h264_nvenc"
	CodecAMF   EncoderCodec = "h264_amf"
	CodecQSV   EncoderCodec = "h264_qsv"
	CodecX264  EncoderCodec = "libx264"

	encoderCacheVersion = 4
	minProbeFPS         = 12.0
)

// probeOrder is hardware-first, software last — works on all user PCs.
var probeOrder = []EncoderCodec{CodecNVENC, CodecAMF, CodecQSV, CodecX264}

func (c EncoderCodec) label() string {
	return "dxgi-" + string(c)
}

func (c EncoderCodec) ffmpegEncodeArgs(prof StreamProfile, outW, outH int) []string {
	bitrate := prof.BitrateK
	tail := []string{
		"-an",
		"-g", fmt.Sprintf("%d", prof.GOP),
		"-keyint_min", fmt.Sprintf("%d", prof.KeyIntMin),
		"-bf", "0",
		"-f", "h264",
		"-flush_packets", "1",
		"pipe:1",
	}

	switch c {
	case CodecNVENC:
		return append([]string{
			"-c:v", string(CodecNVENC),
			"-preset", "p4",
			"-tune", "ll",
			"-profile:v", "baseline",
			"-b:v", fmt.Sprintf("%dk", bitrate),
			"-maxrate", fmt.Sprintf("%dk", bitrate),
			"-bufsize", fmt.Sprintf("%dk", bitrate),
			"-fps_mode", "cfr",
			"-r", fmt.Sprintf("%d", prof.FPS),
		}, tail...)
	case CodecAMF:
		return append([]string{
			"-c:v", string(CodecAMF),
			"-quality", "speed",
			"-usage", "ultralowlatency",
			"-b:v", fmt.Sprintf("%dk", bitrate),
			"-maxrate", fmt.Sprintf("%dk", bitrate),
			"-bufsize", fmt.Sprintf("%dk", bitrate),
			"-fps_mode", "cfr",
			"-r", fmt.Sprintf("%d", prof.FPS),
		}, tail...)
	case CodecQSV:
		return append([]string{
			"-c:v", string(CodecQSV),
			"-look_ahead", "0",
			"-async_depth", "1",
			"-profile:v", "baseline",
			"-preset", "veryfast",
			"-b:v", fmt.Sprintf("%dk", bitrate),
			"-maxrate", fmt.Sprintf("%dk", bitrate),
			"-bufsize", fmt.Sprintf("%dk", bitrate/2),
			"-max_delay", "0",
			"-sc_threshold", "0",
			"-vsync", "0",
		}, tail...)
	default:
		return append([]string{
			"-c:v", string(CodecX264),
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-pix_fmt", "yuv420p",
			"-b:v", fmt.Sprintf("%dk", bitrate),
			"-maxrate", fmt.Sprintf("%dk", bitrate),
			"-bufsize", fmt.Sprintf("%dk", bitrate),
			"-vsync", "0",
		}, tail...)
	}
}
