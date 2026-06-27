package agent

import (
	"log"
	"time"
)

// sessionPerf tracks startup and throughput for one viewer session.
type sessionPerf struct {
	session   string
	started   time.Time
	firstSent time.Time
	sent      int
	lastLog   time.Time
}

func newSessionPerf(session string) *sessionPerf {
	return &sessionPerf{
		session: session,
		started: time.Now(),
		lastLog: time.Now(),
	}
}

func (m *sessionPerf) noteSent() {
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
		log.Printf("agent: perf session %s samples=%d send_fps=%.1f uptime_s=%.0f",
			m.session, m.sent, fps, now.Sub(m.started).Seconds())
		m.lastLog = now
	}
}
