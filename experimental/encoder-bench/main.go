//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"connect/internal/agent"
)

func main() {
	os.Setenv("CONNECT_ALLOW_SYSTEM_FFMPEG", "1")
	prof := agent.DefaultStreamProfile()
	if p, err := findFFmpeg(); err == nil {
		raw := captureMPEGTS(p, prof)
		fmt.Printf("mpegts collect all=%d incremental=%d countAU=%d bytes=%d\n",
			agent.CollectAllFrames(raw),
			agent.CollectAllFramesIncremental(raw, 32768),
			agent.CountTSFrames(raw),
			len(raw))
	}
	fmt.Println("--- agent ReadFrame (15s) ---")
	frames := agentReadFrames()
	fmt.Printf("summary: agent_frames=%d\n", frames)
}

func captureMPEGTS(ffmpegPath string, prof agent.StreamProfile) []byte {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "gdigrab", "-framerate", fmt.Sprintf("%d", prof.FPS),
		"-draw_mouse", "1", "-use_wallclock_as_timestamps", "1", "-t", "15",
		"-i", "desktop", "-an",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear,fps=%d", prof.Width, prof.Height, prof.FPS),
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
		"-profile:v", "baseline", "-pix_fmt", "yuv420p",
		"-b:v", "2000k", "-fps_mode", "cfr", "-r", fmt.Sprintf("%d", prof.FPS),
		"-g", "40", "-bf", "0",
		"-f", "mpegts", "-mpegts_flags", "+resend_headers", "-flush_packets", "1", "pipe:1",
	}
	return rawPipeReadBytes(ffmpegPath, args)
}

func countStartCodes(b []byte) int {
	n := 0
	for i := 0; i+3 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && (b[i+2] == 1 || (b[i+2] == 0 && i+3 < len(b) && b[i+3] == 1)) {
			n++
		}
	}
	return n
}

func dumpNALTypes(b []byte, max int) {
	pos := 0
	for n := 0; n < max; n++ {
		sc := findSC(b, pos)
		if sc < 0 {
			break
		}
		off := sc + 3
		if sc+3 < len(b) && b[sc+2] == 0 {
			off = sc + 4
		}
		if off >= len(b) {
			break
		}
		nt := b[off] & 0x1f
		next := findSC(b, sc+3)
		end := len(b)
		if next > sc {
			end = next
		}
		fmt.Printf("  nal[%d] type=%d at=%d len=%d\n", n, nt, sc, end-sc)
		pos = sc + 4
	}
}

func findSC(b []byte, from int) int {
	for i := from; i+3 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 {
			if b[i+2] == 1 {
				return i
			}
			if i+3 < len(b) && b[i+2] == 0 && b[i+3] == 1 {
				return i
			}
		}
	}
	return -1
}

func findFFmpeg() (string, error) {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("ffmpeg not found")
}

func rawPipeReadBytes(ffmpegPath string, args []string) []byte {
	cmd := exec.Command(ffmpegPath, args...)
	cmd.SysProcAttr = agent.HiddenProcAttr()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("pipe:", err)
		return nil
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("start:", err)
		return nil
	}
	buf := make([]byte, 65536)
	var total []byte
	deadline := time.Now().Add(16 * time.Second)
	for time.Now().Before(deadline) {
		n, err := stdout.Read(buf)
		if n > 0 {
			total = append(total, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return total
}

func agentReadFrames() int {
	cfg := agent.Config{Width: 854, Height: 480, FPS: 20, BitrateK: 2000, GOP: 40, KeyIntMin: 20}
	cfg = agent.NormalizeConfig(cfg)
	enc, err := agent.OpenVideoEncoderBench(cfg, "gdigrab", agent.CodecX264)
	if err != nil {
		fmt.Println("open:", err)
		return 0
	}
	defer enc.Close()
	deadline := time.Now().Add(15 * time.Second)
	frames := 0
	for time.Now().Before(deadline) {
		f, err := enc.ReadFrame()
		if err != nil {
			if err != io.EOF {
				fmt.Println("read:", err)
			}
			break
		}
		if len(f.Data) > 0 {
			frames++
		}
	}
	for {
		f, err := enc.ReadFrame()
		if err != nil {
			break
		}
		if len(f.Data) > 0 {
			frames++
		}
	}
	fmt.Printf("agent ReadFrame: %d frames bytes_in=%d queued=%d\n",
		frames, agent.EncoderBytesIn(enc), agent.EncoderFramesOut(enc))
	return frames
}
