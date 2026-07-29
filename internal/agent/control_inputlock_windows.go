//go:build windows

package agent

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// BlockInput requires UIAccess/elevation and fails with Access Denied for a normal
// interactive agent. Low-level hooks work in the user session and still allow
// SendInput (remote control) through via the INJECTED flag.

const (
	whKeyboardLL   = 13
	whMouseLL      = 14
	llkhfInjected  = 0x00000010
	llmhfInjected  = 0x00000001
	wmQuit         = 0x0012
)

var (
	procSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId  = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

	kbHookCB    = syscall.NewCallback(lowLevelKeyboardProc)
	mouseHookCB = syscall.NewCallback(lowLevelMouseProc)

	hooksWanted   atomic.Bool
	hookThreadID  atomic.Uint32
	kbHookHandle  atomic.Uintptr
	mouseHookHandle atomic.Uintptr

	hookStartMu sync.Mutex
)

type kbdLLHookStruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

type msLLHookStruct struct {
	pt          point
	mouseData   uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

type point struct {
	x, y int32
}

type msgStruct struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

func setBlockInput(block bool) error {
	if block {
		return startInputHooks()
	}
	return stopInputHooks()
}

func startInputHooks() error {
	hookStartMu.Lock()
	defer hookStartMu.Unlock()
	if hooksWanted.Load() {
		return nil
	}

	ready := make(chan error, 1)
	go runInputHookThread(ready)
	if err := <-ready; err != nil {
		return err
	}
	hooksWanted.Store(true)
	return nil
}

func stopInputHooks() error {
	hookStartMu.Lock()
	defer hookStartMu.Unlock()
	hooksWanted.Store(false)

	if h := kbHookHandle.Swap(0); h != 0 {
		procUnhookWindowsHookEx.Call(h)
	}
	if h := mouseHookHandle.Swap(0); h != 0 {
		procUnhookWindowsHookEx.Call(h)
	}
	if tid := hookThreadID.Swap(0); tid != 0 {
		procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
	}
	return nil
}

func runInputHookThread(ready chan<- error) {
	tid, _, _ := procGetCurrentThreadId.Call()
	hookThreadID.Store(uint32(tid))

	kb, _, err := procSetWindowsHookEx.Call(whKeyboardLL, kbHookCB, 0, 0)
	if kb == 0 {
		ready <- errString("input lock hooks failed (keyboard): " + err.Error())
		return
	}
	ms, _, err := procSetWindowsHookEx.Call(whMouseLL, mouseHookCB, 0, 0)
	if ms == 0 {
		procUnhookWindowsHookEx.Call(kb)
		ready <- errString("input lock hooks failed (mouse): " + err.Error())
		return
	}
	kbHookHandle.Store(kb)
	mouseHookHandle.Store(ms)
	ready <- nil

	var msg msgStruct
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	if h := kbHookHandle.Swap(0); h != 0 {
		procUnhookWindowsHookEx.Call(h)
	}
	if h := mouseHookHandle.Swap(0); h != 0 {
		procUnhookWindowsHookEx.Call(h)
	}
	hookThreadID.Store(0)
	hooksWanted.Store(false)
}

func lowLevelKeyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && hooksWanted.Load() && lParam != 0 {
		kbd := (*kbdLLHookStruct)(unsafe.Pointer(lParam))
		if kbd.flags&llkhfInjected == 0 {
			return 1
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

func lowLevelMouseProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && hooksWanted.Load() && lParam != 0 {
		ms := (*msLLHookStruct)(unsafe.Pointer(lParam))
		if ms.flags&llmhfInjected == 0 {
			return 1
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}
