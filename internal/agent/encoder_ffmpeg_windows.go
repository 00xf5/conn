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
	"sync/atomic"
	"time"
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
	outOnce  sync.Once
	out      chan videoFrame
	bytesIn  int64
	framesOut int64
	stash  []byte
	sps    []byte
	pps    []byte
	params []byte // SPS+PPS prepended to delta frames for mobile decoders
	ts     *tsVideoDemux
}

const ffmpegFrameQueue = 128
const minVideoFrameBytes = 200 // P-frames smaller than this are truncated NALs (green/glitch decode)
const minKeyframeBytes = 500  // reject undersized IDRs before they reach the browser

func newFFmpegEncoderBase(cfg Config, codec EncoderCodec, cmd *exec.Cmd, stdout io.ReadCloser) *ffmpegEncoder {
	f := &ffmpegEncoder{
		cfg:    cfg,
		codec:  codec,
		cmd:    cmd,
		stdout: stdout,
		out:    make(chan videoFrame, ffmpegFrameQueue),
		ts:     newTSVideoDemux(),
	}
	f.ready.L = &f.mu
	return f
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

// newGdiGrabFFmpegEncoderBench is like newGdiGrabFFmpegEncoder but limits capture for bench runs.
func newGdiGrabFFmpegEncoderBench(cfg Config, codec EncoderCodec) (*ffmpegEncoder, error) {
	if codec == "" {
		codec = CodecX264
	}
	prof := ProfileFromConfig(cfg)

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer+genpts",
		"-flags", "low_delay",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-thread_queue_size", "512",
		"-f", "gdigrab",
		"-framerate", fmt.Sprintf("%d", prof.FPS),
		"-draw_mouse", "1",
		"-use_wallclock_as_timestamps", "1",
		"-t", "15",
		"-i", "desktop",
		"-an",
		"-vf", gdiGrabVideoFilter(prof),
	}
	args = append(args, codec.ffmpegGdiGrabEncodeArgs(prof, prof.Width, prof.Height)...)

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

	f := newFFmpegEncoderBase(cfg, codec, cmd, stdout)
	go f.readStdoutLoop()
	return f, nil
}

// newGdiGrabFFmpegEncoder captures the desktop via ffmpeg gdigrab + libx264.
func newGdiGrabFFmpegEncoder(cfg Config, codec EncoderCodec) (*ffmpegEncoder, error) {
	if codec == "" {
		codec = CodecX264
	}
	prof := ProfileFromConfig(cfg)

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer+genpts",
		"-flags", "low_delay",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-thread_queue_size", "512",
		"-f", "gdigrab",
		"-framerate", fmt.Sprintf("%d", prof.FPS),
		"-draw_mouse", "1",
		"-use_wallclock_as_timestamps", "1",
		"-i", "desktop",
		"-an",
		"-vf", gdiGrabVideoFilter(prof),
	}
	args = append(args, codec.ffmpegGdiGrabEncodeArgs(prof, prof.Width, prof.Height)...)

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

	f := newFFmpegEncoderBase(cfg, codec, cmd, stdout)
	go f.readStdoutLoop()
	return f, nil
}

// newFFmpegEncoder is an alias for the gdigrab path.
func newFFmpegEncoder(cfg Config) (*ffmpegEncoder, error) {
	return newGdiGrabFFmpegEncoder(cfg, CodecX264)
}

func newFFmpegEncoderWithIO(cfg Config, stdout io.ReadCloser, cmd *exec.Cmd) (*ffmpegEncoder, error) {
	f := newFFmpegEncoderBase(cfg, "", cmd, stdout)
	go f.readStdoutLoop()
	return f, nil
}

func (f *ffmpegEncoder) closeOut() {
	f.outOnce.Do(func() { close(f.out) })
}

func (f *ffmpegEncoder) collectParsedFramesLocked() []videoFrame {
	var frames []videoFrame
	for {
		au, ok, key := f.popAccessUnit()
		if !ok {
			break
		}
		if !acceptVideoFrame(videoFrame{Data: au.data, KeyFrame: key}) {
			continue
		}
		frames = append(frames, videoFrame{Data: au.data, KeyFrame: key})
	}
	return frames
}

func (f *ffmpegEncoder) emitFrames(frames []videoFrame) {
	for _, fr := range frames {
		if len(fr.Data) == 0 {
			continue
		}
		f.out <- fr
		atomic.AddInt64(&f.framesOut, 1)
	}
}

func (f *ffmpegEncoder) FramesOut() int64 { return atomic.LoadInt64(&f.framesOut) }

func (f *ffmpegEncoder) readStdoutLoop() {
	buf := make([]byte, 32768)
	for {
		n, err := f.stdout.Read(buf)
		var frames []videoFrame
		if n > 0 {
			atomic.AddInt64(&f.bytesIn, int64(n))
			_, _ = f.ts.Write(buf[:n])
			frames = f.ts.Drain()
			f.mu.Lock()
			f.ready.Broadcast()
			f.mu.Unlock()
		}
		if err != nil {
			f.mu.Lock()
			f.readErr = err
			f.readDone = true
			frames = append(frames, f.ts.finish()...)
			f.ready.Broadcast()
			f.mu.Unlock()
			f.emitFrames(frames)
			f.closeOut()
			return
		}
		if f.closed {
			f.mu.Lock()
			f.readDone = true
			frames = append(frames, f.ts.finish()...)
			f.ready.Broadcast()
			f.mu.Unlock()
			f.emitFrames(frames)
			f.closeOut()
			return
		}
		f.emitFrames(frames)
	}
}

func (f *ffmpegEncoder) BytesIn() int64 { return atomic.LoadInt64(&f.bytesIn) }

func (f *ffmpegEncoder) Name() string {
	if f.codec != "" {
		return "gdigrab-" + string(f.codec)
	}
	return "gdigrab-libx264"
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
	if f.ts != nil {
		f.ts.Close()
	}
	_ = f.stdout.Close()
	if f.cmd != nil && f.cmd.Process != nil {
		_ = f.cmd.Process.Kill()
	}
	if f.cmd != nil {
		_ = f.cmd.Wait()
	}
	f.closeOut()
	return nil
}

func (f *ffmpegEncoder) ReadFrame() (videoFrame, error) {
	fr, ok := <-f.out
	if ok {
		return fr, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed && !f.readDone {
		return videoFrame{}, fmt.Errorf("encoder closed")
	}
	if f.readErr != nil && f.readErr != io.EOF {
		return videoFrame{}, f.readErr
	}
	return videoFrame{}, io.EOF
}

// popAccessUnit extracts one H.264 access unit (parameter sets + VCL NALs) for WebRTC.
func (f *ffmpegEncoder) popAccessUnit() (accessUnit, bool, bool) {
	var vclParts [][]byte
	hasVCL := false
	isKey := false

	flush := func() (accessUnit, bool, bool) {
		if !hasVCL {
			return accessUnit{}, false, false
		}
		key := isKey
		data := f.packAccessUnit(vclParts, key)
		vclParts = nil
		hasVCL = false
		isKey = false
		if len(data) == 0 {
			return accessUnit{}, false, false
		}
		return accessUnit{data: data}, true, key
	}

	for {
		nal, ok := f.popNextNAL()
		if !ok {
			if hasVCL {
				au, ok2, key2 := flush()
				return au, ok2, key2
			}
			return accessUnit{}, false, false
		}
		nt := nalTypeAt(nal, 0)
		switch nt {
		case 7, 8:
			f.noteParamNAL(nal)
		case 6, 9:
			// skip AUD/SEI
		case 5:
			if hasVCL {
				f.prependNAL(nal)
				return flush()
			}
			vclParts = append(vclParts, nal)
			hasVCL = true
			isKey = true
		case 1, 2:
			if hasVCL {
				f.prependNAL(nal)
				au, ok2, key2 := flush()
				return au, ok2, key2
			}
			vclParts = append(vclParts, nal)
			hasVCL = true
		default:
			// skip other NAL types
		}
	}
}

func (f *ffmpegEncoder) packAccessUnit(vclParts [][]byte, isKey bool) []byte {
	var data []byte
	if isKey && len(f.params) > 0 {
		data = append(data, f.params...)
	}
	for _, p := range vclParts {
		data = append(data, p...)
	}
	return data
}

func (f *ffmpegEncoder) prependNAL(nal []byte) {
	f.stash = append(append([]byte(nil), nal...), f.stash...)
}

func (f *ffmpegEncoder) popNextNAL() ([]byte, bool) {
	for {
		sc := findStartCode(f.stash, 0)
		if sc < 0 {
			return nil, false
		}
		if sc > 0 {
			f.stash = f.stash[sc:]
		}
		hdrSkip := startCodeLen(f.stash, 0)
		if hdrSkip == 0 {
			f.stash = f.stash[1:]
			continue
		}
		next := findStartCode(f.stash, hdrSkip)
		end := len(f.stash)
		if next > 0 {
			end = next
		} else if !f.readDone {
			return nil, false
		}
		nal := append([]byte(nil), f.stash[:end]...)
		f.stash = f.stash[end:]
		if len(nal) > hdrSkip {
			return nal, true
		}
	}
}

func startCodeLen(b []byte, pos int) int {
	if pos+3 >= len(b) {
		return 0
	}
	if b[pos] == 0 && b[pos+1] == 0 && b[pos+2] == 1 {
		return 3
	}
	if pos+4 <= len(b) && b[pos] == 0 && b[pos+1] == 0 && b[pos+2] == 0 && b[pos+3] == 1 {
		return 4
	}
	return 0
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
	for i := from; i+2 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 {
			continue
		}
		if b[i+2] == 1 {
			return i
		}
		if b[i+2] == 3 && i+3 < len(b) {
			i++ // emulation prevention 0x000003
			continue
		}
		if i+3 < len(b) && b[i+2] == 0 && b[i+3] == 1 {
			return i
		}
	}
	return -1
}

// countAnnexBFramesFromReader counts H.264 access units from a live ffmpeg stdout stream.
func countAnnexBFramesFromReader(stdout io.Reader, deadline time.Time) int {
	var parse ffmpegEncoder
	frames := 0
	for time.Now().Before(deadline) {
		chunk := make([]byte, 32768)
		n, readErr := stdout.Read(chunk)
		if n > 0 {
			parse.stash = append(parse.stash, chunk[:n]...)
			for {
				au, ok, key := parse.popAccessUnit()
				if !ok {
					break
				}
				if len(au.data) == 0 {
					continue
				}
				if !acceptVideoFrame(videoFrame{Data: au.data, KeyFrame: key}) {
					continue
				}
				frames++
			}
		}
		if readErr != nil {
			break
		}
	}
	parse.readDone = true
	for {
		au, ok, key := parse.popAccessUnit()
		if !ok {
			break
		}
		if len(au.data) == 0 {
			continue
		}
		if !key && len(au.data) < minVideoFrameBytes {
			continue
		}
		frames++
	}
	return frames
}
