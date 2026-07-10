package agent

import "sync"

// audioRuntime holds session voice I/O state (Windows implements capture/playback).
type audioRuntime struct {
	stop        chan struct{}
	stopOnce    sync.Once
	playMu      sync.Mutex
	playBuf     []int16
	playStarted bool
}
