//go:build !windows

package agent

import (
	"github.com/pion/webrtc/v4"
)

func (a *Agent) startHostMic(track *webrtc.TrackLocalStaticSample, gen uint64) {
	_ = track
	_ = gen
}

func (a *Agent) stopAudioLocked() {}

func (a *Agent) playRemoteAudio(track *webrtc.TrackRemote, gen uint64) {
	_ = track
	_ = gen
}
