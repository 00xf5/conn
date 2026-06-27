package agent

import (
	"log"
	"os"
	"sync"
	"time"
)

// primedEncoder returns a stashed IDR first, then reads from the inner encoder.
type primedEncoder struct {
	inner videoEncoder
	first videoFrame
	mu    sync.Mutex
	sent  bool
}

func (p *primedEncoder) ReadFrame() (videoFrame, error) {
	p.mu.Lock()
	if !p.sent && len(p.first.Data) > 0 {
		p.sent = true
		f := p.first
		p.mu.Unlock()
		return f, nil
	}
	p.mu.Unlock()
	return p.inner.ReadFrame()
}

func (p *primedEncoder) SetBitrate(kbps int) error  { return p.inner.SetBitrate(kbps) }
func (p *primedEncoder) Close() error               { return p.inner.Close() }
func (p *primedEncoder) Name() string               { return p.inner.Name() }
func (p *primedEncoder) CaptureSize() (int, int)    { return p.inner.CaptureSize() }

func primeEncoder(enc videoEncoder, cfg Config) videoEncoder {
	prof := ProfileFromConfig(cfg)
	deadline := time.Now().Add(prof.WarmPrime)
	for time.Now().Before(deadline) {
		f, err := enc.ReadFrame()
		if err != nil {
			break
		}
		if len(f.Data) == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if f.KeyFrame {
			log.Printf("agent: encoder primed with keyframe (%d bytes)", len(f.Data))
			return &primedEncoder{inner: enc, first: f}
		}
	}
	return enc
}

func (a *Agent) preloadEncoder() {
	if os.Getenv("CONNECT_ENCODER_DXGI") == "1" {
		_ = resolveEncoderCodec(a.cfg)
	}
	a.startWarmEncoder()
}

func (a *Agent) startWarmEncoder() {
	a.warmMu.Lock()
	if a.warming || a.warmEnc != nil {
		a.warmMu.Unlock()
		return
	}
	a.mu.Lock()
	busy := a.activeSess != ""
	a.mu.Unlock()
	if busy {
		a.warmMu.Unlock()
		return
	}
	a.warming = true
	a.warmMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("agent: warm encoder panic: %v", r)
			}
		}()

		select {
		case <-a.closed:
			a.warmMu.Lock()
			a.warming = false
			a.warmMu.Unlock()
			return
		default:
		}

		enc, err := openVideoEncoder(a.cfg)
		if err != nil {
			log.Printf("agent: warm encoder failed: %v", err)
			a.warmMu.Lock()
			a.warming = false
			a.warmMu.Unlock()
			return
		}
		enc = primeEncoder(enc, a.cfg)

		a.warmMu.Lock()
		defer a.warmMu.Unlock()
		select {
		case <-a.closed:
			_ = enc.Close()
			a.warming = false
			return
		default:
		}
		a.mu.Lock()
		stillBusy := a.activeSess != ""
		a.mu.Unlock()
		if stillBusy {
			_ = enc.Close()
			a.warming = false
			return
		}
		if a.warmEnc == nil {
			a.warmEnc = enc
			log.Printf("agent: encoder warmed (%s)", enc.Name())
		} else {
			_ = enc.Close()
		}
		a.warming = false
	}()
}

func (a *Agent) takeWarmEncoder() videoEncoder {
	a.warmMu.Lock()
	enc := a.warmEnc
	a.warmEnc = nil
	a.warmMu.Unlock()
	if enc != nil {
		log.Printf("agent: using pre-warmed encoder (%s)", enc.Name())
	}
	return enc
}

func (a *Agent) closeWarmEncoder() {
	a.warmMu.Lock()
	defer a.warmMu.Unlock()
	if a.warmEnc != nil {
		_ = a.warmEnc.Close()
		a.warmEnc = nil
	}
	a.warming = false
}
