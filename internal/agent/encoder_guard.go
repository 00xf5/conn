package agent

import (
	"io"
	"sync"
)

// guardedEncoder serializes ReadFrame and Close on the native HW pipeline.
// Closing the encoder while QSV/DXGI is inside ReadFrame crashes the process (no Go stack trace).
type guardedEncoder struct {
	mu    sync.Mutex
	inner videoEncoder
}

func guardEncoder(enc videoEncoder) videoEncoder {
	if enc == nil {
		return nil
	}
	if _, ok := enc.(*guardedEncoder); ok {
		return enc
	}
	return &guardedEncoder{inner: enc}
}

func (g *guardedEncoder) ReadFrame() (videoFrame, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner == nil {
		return videoFrame{}, io.EOF
	}
	return g.inner.ReadFrame()
}

func (g *guardedEncoder) SetBitrate(kbps int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner == nil {
		return nil
	}
	return g.inner.SetBitrate(kbps)
}

func (g *guardedEncoder) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner == nil {
		return nil
	}
	err := g.inner.Close()
	g.inner = nil
	return err
}

func (g *guardedEncoder) Name() string {
	g.mu.Lock()
	inner := g.inner
	g.mu.Unlock()
	if inner == nil {
		return "closed"
	}
	return inner.Name()
}

func (g *guardedEncoder) CaptureSize() (int, int) {
	g.mu.Lock()
	inner := g.inner
	g.mu.Unlock()
	if inner == nil {
		return 0, 0
	}
	return inner.CaptureSize()
}
