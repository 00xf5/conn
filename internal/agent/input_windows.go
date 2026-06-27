//go:build windows

package agent

import (
	"connect/internal/inputproto"
)

func applyInputWindows(data []byte, capW, capH int) {
	ev, err := inputproto.Decode(data)
	if err != nil {
		return
	}
	injectEvent(ev, capW, capH)
}
