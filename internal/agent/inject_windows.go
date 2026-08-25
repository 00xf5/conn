//go:build windows

package agent

import (
	"syscall"
	"unsafe"

	"connect/internal/inputproto"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSendInput    = user32.NewProc("SendInput")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	eventMouseMove   = 0x0001
	eventMouseLeftDown  = 0x0002
	eventMouseLeftUp    = 0x0004
	eventMouseRightDown = 0x0008
	eventMouseRightUp   = 0x0010
	eventMouseMiddleDown = 0x0020
	eventMouseMiddleUp   = 0x0040
	eventMouseWheel      = 0x0800
	eventKeyUp           = 0x0002
	eventKeyUnicode      = 0x0004

	smCXScreen = 0
	smCYScreen = 1
)

type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type input struct {
	inputType uint32
	_         [4]byte
	mi        mouseInput
}

func screenSize() (int, int) {
	w, _, _ := procGetSystemMetrics.Call(smCXScreen)
	h, _, _ := procGetSystemMetrics.Call(smCYScreen)
	return int(w), int(h)
}

func normToPixel(x, y uint16, _, _ int) (int, int) {
	sw, sh := screenSize()
	// Viewer sends 0–65535 over the stream frame; stream is a scaled full desktop.
	px := int(x) * sw / 65535
	py := int(y) * sh / 65535
	return px, py
}

func injectEvent(ev inputproto.Event, capW, capH int) {
	switch ev.Kind {
	case inputproto.MsgMouseMove:
		px, py := normToPixel(ev.X, ev.Y, capW, capH)
		sendMouseMove(px, py, capW, capH)
	case inputproto.MsgMouseDown, inputproto.MsgMouseUp:
		px, py := normToPixel(ev.X, ev.Y, capW, capH)
		sendMouseMove(px, py, capW, capH)
		flags := mouseButtonFlags(ev.Button, ev.Kind == inputproto.MsgMouseDown)
		if flags != 0 {
			sendMouseButton(flags)
		}
	case inputproto.MsgKeyDown, inputproto.MsgKeyUp:
		sendKey(ev.VK, ev.Kind == inputproto.MsgKeyUp)
	case inputproto.MsgText:
		sendUnicode(ev.Text)
	case inputproto.MsgWheel:
		px, py := normToPixel(ev.X, ev.Y, capW, capH)
		sendMouseMove(px, py, capW, capH)
		sendWheel(int(ev.Delta))
	}
}

func mouseButtonFlags(btn byte, down bool) uint32 {
	switch btn {
	case inputproto.MouseLeft:
		if down {
			return eventMouseLeftDown
		}
		return eventMouseLeftUp
	case inputproto.MouseRight:
		if down {
			return eventMouseRightDown
		}
		return eventMouseRightUp
	case inputproto.MouseMiddle:
		if down {
			return eventMouseMiddleDown
		}
		return eventMouseMiddleUp
	}
	return 0
}

func sendMouseMove(x, y int, capW, capH int) {
	sw, sh := screenSize()
	in := input{
		inputType: inputMouse,
		mi: mouseInput{
			dx:      int32(x * 65535 / maxInt(sw, 1)),
			dy:      int32(y * 65535 / maxInt(sh, 1)),
			dwFlags: eventMouseMove | 0x8000 | 0x4000, // MOVE | ABSOLUTE | VIRTUALDESK
		},
	}
	sendInputs(in)
	_ = capW
	_ = capH
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sendMouseButton(flags uint32) {
	in := input{
		inputType: inputMouse,
		mi:        mouseInput{dwFlags: flags | 0x8000 | 0x4000},
	}
	sendInputs(in)
}

func sendWheel(delta int) {
	in := input{
		inputType: inputMouse,
		mi: mouseInput{
			mouseData: uint32(delta),
			dwFlags:   eventMouseWheel | 0x8000 | 0x4000,
		},
	}
	sendInputs(in)
}

// keyboardInput must be the same size as the Windows INPUT union (40 bytes on
// amd64). SendInput fails outright if cbSize != sizeof(INPUT), so the trailing
// pad is required or no key/text events are injected at all.
type keyboardInput struct {
	inputType uint32
	_         [4]byte
	ki        keybdInput
	_         [8]byte
}

func sendUnicode(s string) {
	for _, r := range s {
		if r == 0 {
			continue
		}
		if r > 0xFFFF {
			hi := uint16(0xD800 + ((r - 0x10000) >> 10))
			lo := uint16(0xDC00 + ((r - 0x10000) & 0x3FF))
			sendUnicodeUnit(hi, false)
			sendUnicodeUnit(lo, false)
			sendUnicodeUnit(lo, true)
			sendUnicodeUnit(hi, true)
			continue
		}
		u := uint16(r)
		sendUnicodeUnit(u, false)
		sendUnicodeUnit(u, true)
	}
}

func sendUnicodeUnit(unit uint16, up bool) {
	flags := uint32(eventKeyUnicode)
	if up {
		flags |= eventKeyUp
	}
	in := keyboardInput{
		inputType: inputKeyboard,
		ki:        keybdInput{wScan: unit, dwFlags: flags},
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func injectText(s string) error {
	if s == "" {
		return errControlInvalid
	}
	if len(s) > 2048 {
		s = s[:2048]
	}
	sendUnicode(s)
	return nil
}

func sendKey(vk uint16, up bool) {
	flags := uint32(0)
	if up {
		flags = eventKeyUp
	}
	in := keyboardInput{
		inputType: inputKeyboard,
		ki:        keybdInput{wVk: vk, dwFlags: flags},
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func sendInputs(in input) {
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}
