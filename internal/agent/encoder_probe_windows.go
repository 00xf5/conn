//go:build windows && cgo

package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

const probeSeconds = 3

var (
	probeOnce     sync.Once
	resolvedCodec EncoderCodec
)

func resolveEncoderCodec(cfg Config) EncoderCodec {
	if v := os.Getenv("CONNECT_ENCODER_CODEC"); v != "" {
		c := EncoderCodec(v)
		for _, ok := range probeOrder {
			if ok == c {
				log.Printf("agent: encoder forced via CONNECT_ENCODER_CODEC=%s", v)
				return c
			}
		}
	}
	if os.Getenv("CONNECT_ENCODER_REPROBE") == "1" {
		_ = os.Remove(encoderCachePath())
		probeOnce = sync.Once{}
	}
	probeOnce.Do(func() {
		if c, ok := loadCachedEncoderCodec(); ok {
			resolvedCodec = c
			return
		}
		resolvedCodec = probeBestCodec(cfg)
	})
	return resolvedCodec
}

func probeBestCodec(cfg Config) EncoderCodec {
	prof := ProfileFromConfig(cfg)
	log.Printf("agent: probing encoders with live DXGI (%dx%d, need >= %.0f fps)...", prof.Width, prof.Height, minProbeFPS)

	capCfg := cfg
	capCfg.Width = prof.Width
	capCfg.Height = prof.Height

	cap, w, h, err := openDXGICapture(capCfg)
	if err != nil {
		log.Printf("agent: dxgi probe capture failed (%v); cache %s", err, CodecX264)
		saveCachedEncoderCodec(CodecX264, 0)
		return CodecX264
	}
	defer cap.Close()

	var bestHW EncoderCodec
	var bestHWFPS float64
	var libx264FPS float64
	var hasLibx264 bool

	for _, codec := range probeOrder {
		fps, ok := probeCodecLive(codec, prof, cap, w, h)
		if !ok {
			log.Printf("agent: probe %s: unavailable", codec)
			continue
		}
		log.Printf("agent: probe %s: %.1f fps", codec, fps)
		if codec == CodecX264 {
			hasLibx264 = true
			libx264FPS = fps
			continue
		}
		if fps > bestHWFPS {
			bestHW = codec
			bestHWFPS = fps
		}
		if fps >= minProbeFPS {
			saveCachedEncoderCodec(codec, fps)
			return codec
		}
	}

	// libx264 often wins short probes but stalls on the live DXGI pipe; prefer any working GPU path.
	if bestHWFPS > 0 {
		log.Printf("agent: using hardware %s (%.1f fps)", bestHW, bestHWFPS)
		saveCachedEncoderCodec(bestHW, bestHWFPS)
		return bestHW
	}
	if hasLibx264 {
		log.Printf("agent: no hardware encoder; using libx264 (%.1f fps)", libx264FPS)
		saveCachedEncoderCodec(CodecX264, libx264FPS)
		return CodecX264
	}

	log.Printf("agent: probe fallback %s", CodecX264)
	saveCachedEncoderCodec(CodecX264, 0)
	return CodecX264
}

func probeCodecLive(codec EncoderCodec, prof StreamProfile, cap *captureencOnly, w, h int) (float64, bool) {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return 0, false
	}

	streamW, streamH := prof.Width, prof.Height
	fps := prof.FPS

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer+genpts+discardcorrupt",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-f", "rawvideo", "-pix_fmt", "nv12",
		"-s", fmt.Sprintf("%dx%d", w, h),
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", "pipe:0",
	}
	if w != streamW || h != streamH {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", streamW, streamH))
	}
	args = append(args, codec.ffmpegEncodeArgs(prof, streamW, streamH)...)

	cmd := exec.Command(ffmpegPath, args...)
	prepareHiddenCmd(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, false
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, false
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return 0, false
	}
	go drainFFmpegStderr(stderr, "probe "+string(codec))

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer stdin.Close()
		interval := time.Second / time.Duration(fps)
		if interval <= 0 {
			interval = time.Second / 20
		}
		deadline := time.Now().Add(probeSeconds * time.Second)
		next := time.Now()
		for time.Now().Before(deadline) {
			frame, err := cap.ReadNV12()
			if err != nil || len(frame.Data) == 0 {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			if err := writeFull(stdin, frame.Data); err != nil {
				return
			}
			next = next.Add(interval)
			if sleep := time.Until(next); sleep > 0 {
				time.Sleep(sleep)
			} else {
				next = time.Now()
			}
		}
	}()

	deadline := time.Now().Add(probeSeconds*time.Second + 500*time.Millisecond)
	reader := &ffmpegEncoder{}
	frames := 0
	for time.Now().Before(deadline) {
		chunk := make([]byte, 32768)
		n, readErr := stdout.Read(chunk)
		if n > 0 {
			reader.stash = append(reader.stash, chunk[:n]...)
			for {
				au, ok, _ := reader.popAccessUnit()
				if !ok {
					break
				}
				_ = au
				frames++
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			break
		}
	}
	<-done
	_ = stdout.Close()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()

	if frames == 0 {
		return 0, false
	}
	return float64(frames) / float64(probeSeconds), true
}
