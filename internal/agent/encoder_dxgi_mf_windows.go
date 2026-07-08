//go:build windows && cgo

package agent

import (
	"fmt"
	"log"
	"sync"
	"time"

	"connect/internal/captureenc"
)

type dxgiMFEncoder struct {
	cap     *captureencOnly
	enc     *captureenc.MFH264Encoder
	mu      sync.Mutex
	closed  bool
	fps     int
	streamW int
	streamH int
	out     chan videoFrame
}

func newDXGIMFEncoder(cfg Config) (*dxgiMFEncoder, error) {
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

	enc, err := captureenc.NewMFH264Encoder(streamW, streamH, prof.FPS, prof.BitrateK)
	if err != nil {
		_ = cap.Close()
		return nil, err
	}

	e := &dxgiMFEncoder{
		cap: cap, enc: enc, fps: prof.FPS,
		streamW: streamW, streamH: streamH,
		out: make(chan videoFrame, 32),
	}
	go e.captureLoop(w, h)
	log.Printf("agent: dxgi %dx%d -> %s stream %dx%d @ %dkbps", w, h, enc.Name(), streamW, streamH, prof.BitrateK)
	return e, nil
}

func (d *dxgiMFEncoder) captureLoop(captureW, captureH int) {
	interval := time.Second / time.Duration(d.fps)
	if interval <= 0 {
		interval = time.Second / 20
	}
	next := time.Now()

	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			close(d.out)
			return
		}
		cap := d.cap
		enc := d.enc
		d.mu.Unlock()
		if cap == nil || enc == nil {
			close(d.out)
			return
		}

		frame, err := cap.ReadNV12()
		if err != nil {
			log.Printf("agent: dxgi mf capture stopped: %v", err)
			d.mu.Lock()
			d.closed = true
			d.mu.Unlock()
			close(d.out)
			return
		}
		if len(frame.Data) == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		pitch := frame.Pitch
		if pitch <= 0 {
			pitch = captureW
		}
		pkt, err := enc.EncodeNV12(frame.Data, pitch, frame.Width, frame.Height)
		if err != nil {
			log.Printf("agent: mf encode: %v", err)
			continue
		}
		if len(pkt.Data) == 0 {
			next = next.Add(interval)
			if sleep := time.Until(next); sleep > 0 {
				time.Sleep(sleep)
			} else {
				next = time.Now()
			}
			continue
		}
		if err := captureenc.ValidateH264AccessUnit(pkt.Data, pkt.KeyFrame); err != nil {
			continue
		}

		vf := videoFrame{Data: pkt.Data, KeyFrame: pkt.KeyFrame}
		select {
		case d.out <- vf:
		default:
			select {
			case <-d.out:
			default:
			}
			select {
			case d.out <- vf:
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

func (d *dxgiMFEncoder) CaptureSize() (int, int) {
	return d.streamW, d.streamH
}

func (d *dxgiMFEncoder) Name() string {
	if d.enc != nil {
		return "dxgi-" + d.enc.Name()
	}
	return "dxgi-mf-h264"
}

func (d *dxgiMFEncoder) ReadFrame() (videoFrame, error) {
	fr, ok := <-d.out
	if ok {
		return fr, nil
	}
	return videoFrame{}, fmt.Errorf("encoder closed")
}

func (d *dxgiMFEncoder) SetBitrate(kbps int) error {
	d.mu.Lock()
	enc := d.enc
	d.mu.Unlock()
	if enc == nil {
		return nil
	}
	return enc.SetBitrate(kbps)
}

func (d *dxgiMFEncoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	if d.enc != nil {
		_ = d.enc.Close()
		d.enc = nil
	}
	if d.cap != nil {
		_ = d.cap.Close()
		d.cap = nil
	}
	return nil
}
