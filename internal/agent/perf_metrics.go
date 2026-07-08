package agent

import (
	"log"
	"sync"
	"time"
)

// sessionPerf tracks startup and throughput for one viewer session.
type sessionPerf struct {
	session  string
	pipeline string
	started  time.Time

	mu        sync.Mutex
	firstSent time.Time
	sent      int
	dropped   int
	rejected  int
	skipped   int // non-keyframes skipped while waiting for IDR
	lastLog   time.Time
}

func newSessionPerf(session, pipeline string) *sessionPerf {
	return &sessionPerf{
		session:  session,
		pipeline: pipeline,
		started:  time.Now(),
		lastLog:  time.Now(),
	}
}

func (m *sessionPerf) noteSent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sent++
	if m.firstSent.IsZero() {
		m.firstSent = now
		log.Printf("agent: perf session %s first_frame_ms=%d",
			m.session, now.Sub(m.started).Milliseconds())
	}
	if m.sent <= 3 || now.Sub(m.lastLog) >= 60*time.Second {
		elapsed := now.Sub(m.firstSent)
		fps := 0.0
		if elapsed > 0 && m.sent > 1 {
			fps = float64(m.sent-1) / elapsed.Seconds()
		}
		log.Printf("agent: perf session %s samples=%d send_fps=%.1f uptime_s=%.0f dropped=%d rejected=%d skipped_nonkey=%d",
			m.session, m.sent, fps, now.Sub(m.started).Seconds(), m.dropped, m.rejected, m.skipped)
		m.lastLog = now
	}
}

func (m *sessionPerf) noteDropped() {
	m.mu.Lock()
	m.dropped++
	m.mu.Unlock()
}

func (m *sessionPerf) noteRejected() {
	m.mu.Lock()
	m.rejected++
	m.mu.Unlock()
}

func (m *sessionPerf) noteSkippedNonKey() {
	m.mu.Lock()
	m.skipped++
	m.mu.Unlock()
}

func (m *sessionPerf) logSummary(stalled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	connected := m.sent > 0
	duration := time.Since(m.started).Seconds()
	fps := 0.0
	if !m.firstSent.IsZero() && m.sent > 1 {
		elapsed := time.Since(m.firstSent).Seconds()
		if elapsed > 0 {
			fps = float64(m.sent-1) / elapsed
		}
	}
	log.Printf("agent: session %s summary connected=%t pipeline=%s send_fps=%.1f stalled=%t duration_s=%.0f dropped=%d rejected=%d skipped_nonkey=%d",
		m.session, connected, m.pipeline, fps, stalled, duration, m.dropped, m.rejected, m.skipped)
}

func (m *sessionPerf) avgSendFPS() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.firstSent.IsZero() || m.sent <= 1 {
		return 0
	}
	elapsed := time.Since(m.firstSent).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(m.sent-1) / elapsed
}
