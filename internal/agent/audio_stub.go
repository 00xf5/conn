//go:build !windows

package agent

import (
	"github.com/pion/webrtc/v4"
)

func (a *Agent) ensureAmbientMic() {}

func (a *Agent) stopAmbientMicLocked() {}

func (a *Agent) stopSessionAudioLocked() {}

func (a *Agent) stopAudioLocked() {
	a.stopSessionAudioLocked()
	a.stopAmbientMicLocked()
}

func (a *Agent) audioLevel() float64 { return 0 }

func (a *Agent) attachHostMicTrack(track *webrtc.TrackLocalStaticSample, gen uint64) {
	_ = track
	_ = gen
}

func (a *Agent) playRemoteAudio(track *webrtc.TrackRemote, gen uint64) {
	_ = track
	_ = gen
}
