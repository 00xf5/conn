package agent

import "fmt"

// EncoderCodec labels the ffmpeg video encoder (v1: libx264 only).
type EncoderCodec string

const CodecX264 EncoderCodec = "libx264"

func (c EncoderCodec) label() string {
	return "dxgi-" + string(c)
}

func gdiGrabVideoFilter(prof StreamProfile) string {
	return fmt.Sprintf("scale=%d:%d:flags=fast_bilinear,fps=%d", prof.Width, prof.Height, prof.FPS)
}

func (c EncoderCodec) ffmpegEncodeArgs(prof StreamProfile, outW, outH int) []string {
	return ffmpegLibx264Args(prof, false)
}

// ffmpegGdiGrabEncodeArgs forces CFR timestamps — gdigrab wall-clock capture otherwise
// triggers x264 "non monotonically increasing dts" and single-digit send_fps.
func (c EncoderCodec) ffmpegGdiGrabEncodeArgs(prof StreamProfile, outW, outH int) []string {
	return ffmpegLibx264Args(prof, true)
}

func ffmpegLibx264Args(prof StreamProfile, gdigrabCFR bool) []string {
	bitrate := prof.BitrateK
	tail := []string{
		"-an",
		"-g", fmt.Sprintf("%d", prof.GOP),
		"-keyint_min", fmt.Sprintf("%d", prof.KeyIntMin),
		"-bf", "0",
		"-f", "mpegts",
		"-mpegts_flags", "+resend_headers",
		"-flush_packets", "1",
		"pipe:1",
	}
	x264 := []string{
		"-c:v", string(CodecX264),
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-profile:v", "baseline",
		"-pix_fmt", "yuv420p",
		"-b:v", fmt.Sprintf("%dk", bitrate),
		"-maxrate", fmt.Sprintf("%dk", bitrate),
		"-bufsize", fmt.Sprintf("%dk", bitrate),
		"-fps_mode", "cfr",
		"-r", fmt.Sprintf("%d", prof.FPS),
		"-x264-params", "repeat-headers=1:scenecut=0:slices=1",
		"-level", "3.1",
	}
	_ = gdigrabCFR // CFR is always on for v1; gdigrab path uses fps filter upstream.
	return append(x264, tail...)
}
