package agent

import (
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	termMaxInBytes  = 8 << 10
	termMaxOutBurst = 12 << 10
	termOutInterval = 20 * time.Millisecond
)

type hostShell struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	resize func(cols, rows int) error
	kill   func()
}

type termSession struct {
	stdin  io.WriteCloser
	resize func(cols, rows int) error
	kill   func()
	stopCh chan struct{}
	dc     *webrtc.DataChannel
}

var (
	termMu sync.Mutex
	term   *termSession
)

func termOpen(dc *webrtc.DataChannel, cols, rows int) error {
	termMu.Lock()
	defer termMu.Unlock()
	if term != nil {
		termCloseLocked()
	}
	sh, err := startHostShell(cols, rows)
	if err != nil {
		return err
	}
	st := &termSession{
		stdin:  sh.stdin,
		resize: sh.resize,
		kill:   sh.kill,
		stopCh: make(chan struct{}),
		dc:     dc,
	}
	term = st
	go termPumpOut(st, sh.stdout)
	log.Printf("agent: terminal opened (conpty %dx%d)", cols, rows)
	return nil
}

func termWrite(text string) error {
	if len(text) > termMaxInBytes {
		return errString("input too large")
	}
	termMu.Lock()
	st := term
	termMu.Unlock()
	if st == nil || st.stdin == nil {
		return errString("terminal not open")
	}
	_, err := io.WriteString(st.stdin, text)
	return err
}

func termResize(cols, rows int) error {
	termMu.Lock()
	st := term
	termMu.Unlock()
	if st == nil || st.resize == nil {
		return errString("terminal not open")
	}
	return st.resize(cols, rows)
}

func termClose() error {
	termMu.Lock()
	defer termMu.Unlock()
	if term == nil {
		return nil
	}
	termCloseLocked()
	return nil
}

func forceTermClose() {
	termMu.Lock()
	defer termMu.Unlock()
	if term == nil {
		return
	}
	termCloseLocked()
}

func termCloseLocked() {
	st := term
	term = nil
	if st == nil {
		return
	}
	select {
	case <-st.stopCh:
	default:
		close(st.stopCh)
	}
	if st.kill != nil {
		st.kill()
	} else if st.stdin != nil {
		_ = st.stdin.Close()
	}
	log.Printf("agent: terminal closed")
}

func termPumpOut(st *termSession, r io.ReadCloser) {
	defer func() {
		_ = r.Close()
		termMu.Lock()
		still := term == st
		if still {
			term = nil
		}
		termMu.Unlock()
		if still {
			termSend(st.dc, map[string]any{
				"type":   "control_result",
				"action": "term_exit",
				"ok":     true,
			})
		}
	}()

	buf := make([]byte, 4096)
	var pending []byte
	ticker := time.NewTicker(termOutInterval)
	defer ticker.Stop()

	readCh := make(chan readChunk, 8)
	go func() {
		defer close(readCh)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				select {
				case readCh <- readChunk{b: cp}:
				case <-st.stopCh:
					return
				}
			}
			if err != nil {
				select {
				case readCh <- readChunk{err: err}:
				case <-st.stopCh:
				}
				return
			}
		}
	}()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		chunk := pending
		pending = nil
		termSend(st.dc, map[string]any{
			"type":   "control_result",
			"action": "term_out",
			"ok":     true,
			"data":   string(chunk),
		})
	}

	for {
		select {
		case <-st.stopCh:
			flush()
			return
		case <-ticker.C:
			flush()
		case ch, ok := <-readCh:
			if !ok || ch.err != nil {
				flush()
				return
			}
			pending = append(pending, ch.b...)
			if len(pending) >= termMaxOutBurst {
				flush()
			}
		}
	}
}

type readChunk struct {
	b   []byte
	err error
}

func termSend(dc *webrtc.DataChannel, payload map[string]any) {
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = dc.SendText(string(raw))
}
