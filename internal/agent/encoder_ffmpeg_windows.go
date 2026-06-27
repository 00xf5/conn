//go:build windows

package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type ffmpegEncoder struct {
	cfg    Config
	codec  EncoderCodec
	cmd    *exec.Cmd
	stdout io.ReadCloser
	mu     sync.Mutex
	ready  sync.Cond
	closed bool
	readDone bool
	readErr  error
	stash  []byte
	sps    []byte
	pps    []byte
	params []byte // SPS+PPS prepended to delta frames for mobile decoders
}

// bundledFFmpegPaths returns candidate ffmpeg.exe paths next to connect-agent (production layout).
func bundledFFmpegPaths() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dir := filepath.Dir(exe)
	return []string{
		filepath.Join(dir, "bin", "ffmpeg.exe"),
		filepath.Join(dir, "ffmpeg.exe"),
	}
}

func findFFmpeg() (string, error) {
	for _, p := range bundledFFmpegPaths() {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Dev-only fallbacks (installed host must ship bin/ffmpeg.exe beside connect-agent.exe).
	if os.Getenv("CONNECT_ALLOW_SYSTEM_FFMPEG") == "1" {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			log.Printf("agent: using system ffmpeg (dev): %s", p)
			return p, nil
		}
		glob := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Packages", "Gyan.FFmpeg_*", "*", "bin", "ffmpeg.exe")
		matches, _ := filepath.Glob(glob)
		for _, m := range matches {
			if _, err := os.Stat(m); err == nil {
				log.Printf("agent: using winget ffmpeg (dev): %s", m)
				return m, nil
			}
		}
	}
	return "", fmt.Errorf("ffmpeg not found — reinstall Connect (missing bin\\ffmpeg.exe next to connect-agent.exe)")
}

// newGdiGrabFFmpegEncoder captures the desktop via ffmpeg gdigrab + H.264 (stable default on Windows).
func newGdiGrabFFmpegEncoder(cfg Config, codec EncoderCodec) (*ffmpegEncoder, error) {
	if codec == "" {
		codec = CodecQSV
	}
	prof := ProfileFromConfig(cfg)

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer+genpts",
		"-f", "gdigrab",
		"-framerate", fmt.Sprintf("%d", prof.FPS),
		"-draw_mouse", "1",
		"-i", "desktop",
		"-an",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", prof.Width, prof.Height),
	}
	args = append(args, codec.ffmpegEncodeArgs(prof, prof.Width, prof.Height)...)

	cmd := exec.Command(ffmpegPath, args...)
	prepareHiddenCmd(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg gdigrab start: %w", err)
	}
	go drainFFmpegStderr(stderr, "gdigrab-"+string(codec))

	f := &ffmpegEncoder{cfg: cfg, codec: codec, cmd: cmd, stdout: stdout}
	f.ready.L = &f.mu
	go f.readStdoutLoop()
	return f, nil
}

// newFFmpegEncoder is an alias for the default gdigrab path.
func newFFmpegEncoder(cfg Config) (*ffmpegEncoder, error) {
	return newGdiGrabFFmpegEncoder(cfg, CodecQSV)
}

func newFFmpegEncoderWithIO(cfg Config, stdout io.ReadCloser, cmd *exec.Cmd) (*ffmpegEncoder, error) {
	f := &ffmpegEncoder{cfg: cfg, cmd: cmd, stdout: stdout}
	f.ready.L = &f.mu
	go f.readStdoutLoop()
	return f, nil
}

func (f *ffmpegEncoder) readStdoutLoop() {
	buf := make([]byte, 32768)
	for {
		n, err := f.stdout.Read(buf)
		f.mu.Lock()
		if n > 0 {
			f.stash = append(f.stash, buf[:n]...)
			f.ready.Broadcast()
		}
		if err != nil {
			f.readErr = err
			f.readDone = true
			f.ready.Broadcast()
			f.mu.Unlock()
			return
		}
		if f.closed {
			f.readDone = true
			f.ready.Broadcast()
			f.mu.Unlock()
			return
		}
		f.mu.Unlock()
	}
}

func (f *ffmpegEncoder) Name() string {
	if f.codec != "" {
		return "ffmpeg-gdigrab-" + string(f.codec)
	}
	return "ffmpeg-gdigrab-qsv"
}

func (f *ffmpegEncoder) CaptureSize() (int, int) {
	prof := ProfileFromConfig(f.cfg)
	return prof.Width, prof.Height
}

func (f *ffmpegEncoder) SetBitrate(kbps int) error {
	_ = kbps
	return nil
}

func (f *ffmpegEncoder) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.mu.Unlock()
	_ = f.stdout.Close()
	if f.cmd != nil && f.cmd.Process != nil {
		_ = f.cmd.Process.Kill()
	}
	if f.cmd != nil {
		_ = f.cmd.Wait()
	}
	return nil
}

func (f *ffmpegEncoder) ReadFrame() (videoFrame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return videoFrame{}, fmt.Errorf("encoder closed")
	}
	for {
		if au, key, ok := f.popAccessUnit(); ok {
			return videoFrame{Data: au.data, KeyFrame: key}, nil
		}
		if f.readDone {
			if f.readErr != nil && f.readErr != io.EOF {
				return videoFrame{}, f.readErr
			}
			return videoFrame{}, io.EOF
		}
		f.ready.Wait()
	}
}

// popAccessUnit bundles parameter sets with slices and prepends SPS/PPS on delta frames for mobile.
func (f *ffmpegEncoder) popAccessUnit() (accessUnit, bool, bool) {
	sc := findStartCode(f.stash, 0)
	if sc < 0 {
		return accessUnit{}, false, false
	}

	pos := sc
	key := false
	for {
		next := findStartCode(f.stash, pos+3)
		nt := nalTypeAt(f.stash, pos)
		end := len(f.stash)
		if next >= 0 {
			end = next
		}

		if nt == 7 || nt == 8 {
			f.noteParamNAL(f.stash[pos:end])
		}

		if nt == 5 {
			key = true
			data := append([]byte(nil), f.stash[sc:end]...)
			if next < 0 {
				f.stash = nil
			} else {
				f.stash = f.stash[end:]
			}
			return accessUnit{data: data}, true, true
		}
		if nt == 1 || nt == 2 {
			if next < 0 {
				if len(f.stash)-sc < 8 {
					return accessUnit{}, false, false
				}
				data := append([]byte(nil), f.stash[sc:]...)
				f.stash = nil
				if len(f.params) > 0 && !key {
					data = append(append([]byte(nil), f.params...), data...)
				}
				return accessUnit{data: data}, true, key
			}
			data := append([]byte(nil), f.stash[sc:end]...)
			if len(f.params) > 0 && !key {
				data = append(append([]byte(nil), f.params...), data...)
			}
			f.stash = f.stash[end:]
			return accessUnit{data: data}, true, key
		}

		if next < 0 {
			return accessUnit{}, false, false
		}
		pos = next
	}
}

func (f *ffmpegEncoder) noteParamNAL(nal []byte) {
	switch nalTypeAt(nal, 0) {
	case 7:
		f.sps = append([]byte(nil), nal...)
	case 8:
		f.pps = append([]byte(nil), nal...)
	default:
		return
	}
	if len(f.sps) > 0 && len(f.pps) > 0 {
		f.params = append(append([]byte(nil), f.sps...), f.pps...)
	}
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

type accessUnit struct {
	data []byte
}

// popAccessUnit is kept for tests; use (*ffmpegEncoder).popAccessUnit in production.
func popAccessUnit(buf []byte) (accessUnit, bool, bool) {
	f := &ffmpegEncoder{stash: buf}
	au, ok, key := f.popAccessUnit()
	if !ok {
		return accessUnit{}, false, false
	}
	return accessUnit{data: au.data}, ok, key
}

func nalTypeAt(buf []byte, sc int) byte {
	off := sc + 3
	if sc+3 < len(buf) && buf[sc+2] == 0 {
		off = sc + 4
	}
	if off >= len(buf) {
		return 0
	}
	return buf[off] & 0x1f
}

func findStartCode(b []byte, from int) int {
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
