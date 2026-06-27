//go:build windows && cgo

package agent

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

type dxgiFFmpegEncoder struct {
	cap     *captureencOnly
	ffmpeg  *ffmpegEncoder
	codec   EncoderCodec
	mu      sync.Mutex
	closed  bool
	fps     int
	streamW int
	streamH int
	stdinCh chan []byte
}

func newDXGIFFmpegEncoder(cfg Config, codec EncoderCodec) (*dxgiFFmpegEncoder, error) {
	if codec == "" {
		codec = CodecX264
	}
	prof := ProfileFromConfig(cfg)
	streamW, streamH := prof.Width, prof.Height
	capCfg := cfg
	capCfg.Width = streamW
	capCfg.Height = streamH

	cap, w, h, err := openDXGICapture(capCfg)
	if err != nil {
		return nil, err
	}
	if w <= 0 || h <= 0 {
		w, h = streamW, streamH
	}

	fps := prof.FPS
	bitrate := prof.BitrateK

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		cap.Close()
		return nil, err
	}

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
		cap.Close()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cap.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cap.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cap.Close()
		return nil, err
	}
	go drainFFmpegStderr(stderr, codec.label())

	ff, err := newFFmpegEncoderWithIO(cfg, stdout, cmd)
	if err != nil {
		cap.Close()
		return nil, err
	}

	e := &dxgiFFmpegEncoder{
		cap: cap, ffmpeg: ff, codec: codec, fps: fps,
		streamW: streamW, streamH: streamH,
		stdinCh: make(chan []byte, 32),
	}
	go e.pumpCapture()
	go e.pumpWriter(stdin)
	log.Printf("agent: dxgi %dx%d -> %s stream %dx%d @ %dkbps", w, h, codec, streamW, streamH, bitrate)
	return e, nil
}

func drainFFmpegStderr(r io.ReadCloser, label string) {
	defer r.Close()
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			log.Printf("agent: %s: %s", label, string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

func (d *dxgiFFmpegEncoder) pumpCapture() {
	interval := time.Second / time.Duration(d.fps)
	if interval <= 0 {
		interval = time.Second / 20
	}
	next := time.Now()
	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return
		}
		cap := d.cap
		d.mu.Unlock()
		if cap == nil {
			return
		}

		frame, err := cap.ReadNV12()
		if err != nil {
			log.Printf("agent: dxgi capture pump stopped: %v", err)
			d.mu.Lock()
			if d.ffmpeg != nil {
				_ = d.ffmpeg.Close()
			}
			d.mu.Unlock()
			return
		}
		if len(frame.Data) == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		data := frame.Data
		select {
		case d.stdinCh <- data:
		default:
			select {
			case <-d.stdinCh:
			default:
			}
			select {
			case d.stdinCh <- data:
			default:
			}
		}

		next = next.Add(interval)
		if sleep := time.Until(next); sleep > 0 {
			time.Sleep(sleep)
		} else {
			next = time.Now()
		}
	}
}

func (d *dxgiFFmpegEncoder) pumpWriter(stdin io.WriteCloser) {
	defer stdin.Close()
	for data := range d.stdinCh {
		if err := writeFull(stdin, data); err != nil {
			log.Printf("agent: dxgi capture pump write failed: %v", err)
			return
		}
	}
}

func (d *dxgiFFmpegEncoder) CaptureSize() (int, int) {
	return d.streamW, d.streamH
}

func (d *dxgiFFmpegEncoder) Name() string {
	return d.codec.label()
}

func (d *dxgiFFmpegEncoder) ReadFrame() (videoFrame, error) {
	d.mu.Lock()
	ff := d.ffmpeg
	d.mu.Unlock()
	if ff == nil {
		return videoFrame{}, fmt.Errorf("encoder closed")
	}
	return ff.ReadFrame()
}

func (d *dxgiFFmpegEncoder) SetBitrate(kbps int) error { _ = kbps; return nil }

func (d *dxgiFFmpegEncoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	close(d.stdinCh)
	if d.ffmpeg != nil {
		_ = d.ffmpeg.Close()
	}
	if d.cap != nil {
		_ = d.cap.Close()
	}
	return nil
}
