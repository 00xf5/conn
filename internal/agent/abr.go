package agent

import (
	"log"
	"time"
)

const (
	abrMinInterval     = 3 * time.Second
	abrNativeInterval  = 5 * time.Second
	abrStepDown        = 400
	abrStepUp          = 250
	abrManualHold      = 45 * time.Second
	abrLossDown        = 0.04
	abrLossUp          = 0.012
	abrRTTDownMs       = 180
	abrRTTUpMs         = 90
)

func (a *Agent) resetABRLocked() {
	a.abrBitrateK = 0
	a.abrLastAdjust = time.Time{}
	a.abrHoldUntil = time.Time{}
}

func (a *Agent) liveBitrateLocked() int {
	prof := ProfileFromConfig(a.cfg)
	if a.abrBitrateK <= 0 {
		a.abrBitrateK = prof.BitrateK
	}
	return prof.ClampBitrate(a.abrBitrateK)
}

// adaptBitrateFromStats gently steps bitrate from viewer loss/RTT.
// Manual set_bitrate holds ABR off briefly so the slider stays authoritative.
func (a *Agent) adaptBitrateFromStats(loss, rttMs float64, mobile bool) {
	a.mu.Lock()
	enc := a.enc
	if enc == nil || a.activeSess == "" {
		a.mu.Unlock()
		return
	}
	if time.Now().Before(a.abrHoldUntil) {
		a.mu.Unlock()
		return
	}
	_, native := enc.(*hostPipelineEncoder)
	minGap := abrMinInterval
	if native {
		minGap = abrNativeInterval
	}
	if !a.abrLastAdjust.IsZero() && time.Since(a.abrLastAdjust) < minGap {
		a.mu.Unlock()
		return
	}

	prof := ProfileFromConfig(a.cfg)
	cur := a.liveBitrateLocked()
	ceiling := prof.BitrateK
	if mobile && ceiling > 4500 {
		ceiling = 4500
	}
	floor := prof.BitrateMin
	if mobile && floor < 1500 {
		floor = 1500
	}

	next := cur
	switch {
	case loss >= abrLossDown || rttMs >= float64(abrRTTDownMs):
		next = cur - abrStepDown
	case loss <= abrLossUp && rttMs > 0 && rttMs <= float64(abrRTTUpMs) && cur < ceiling:
		next = cur + abrStepUp
	default:
		a.mu.Unlock()
		return
	}
	if next < floor {
		next = floor
	}
	if next > ceiling {
		next = ceiling
	}
	if next > prof.BitrateMax {
		next = prof.BitrateMax
	}
	if next == cur {
		a.mu.Unlock()
		return
	}
	a.abrBitrateK = next
	a.abrLastAdjust = time.Now()
	sess := a.activeSess
	a.mu.Unlock()

	if err := enc.SetBitrate(next); err != nil {
		log.Printf("agent: abr set bitrate %d: %v", next, err)
		return
	}
	if sess != "" {
		log.Printf("agent: abr session=%s bitrate=%dkbps loss=%.1f%% rtt=%.0fms mobile=%t",
			sess, next, loss*100, rttMs, mobile)
	}
}
