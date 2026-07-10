package agent

import "sync"

// audioRuntime holds ambient mic capture, level metering, and session playback.
type audioRuntime struct {
	ambientStop chan struct{}
	ambientOnce sync.Once
	micStarted  bool

	capMu   sync.Mutex
	pending []int16
	level   float64 // 0..1 RMS

	playMu      sync.Mutex
	playBuf     []int16
	playStarted bool
}
