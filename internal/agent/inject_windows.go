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

type keyboardInput struct {
	inputType uint32
	ki        keybdInput
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
