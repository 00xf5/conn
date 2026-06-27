//go:build windows

package agent

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

func controlCtrlAltDel() error {
	sas := syscall.NewLazyDLL("sas.dll")
	proc := sas.NewProc("SendSAS")
	r, _, _ := proc.Call(0)
	if r != 0 {
		return nil
	}
	return errControlInvalid
}

func controlLock() error {
	proc := user32.NewProc("LockWorkStation")
	r, _, _ := proc.Call()
	if r == 0 {
		return errControlInvalid
	}
	return nil
}

func controlShutdown(reboot bool) error {
	flag := "/s"
	if reboot {
		flag = "/r"
	}
	return exec.Command("shutdown", flag, "/t", "5", "/c", "Connect remote session").Run()
}

func controlOpenURL(url string) error {
	if url == "" {
		return errControlInvalid
	}
	return exec.Command("cmd", "/c", "start", "", url).Start()
}

func controlRun(cmd string) error {
	if cmd == "" {
		return errControlInvalid
	}
	return exec.Command("cmd", "/c", "start", "", "cmd", "/c", cmd).Start()
}

func controlClipboard(text string) error {
	if text == "" {
		return errControlInvalid
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	open := user32.NewProc("OpenClipboard")
	closeClip := user32.NewProc("CloseClipboard")
	empty := user32.NewProc("EmptyClipboard")
	setData := user32.NewProc("SetClipboardData")
	globalAlloc := kernel32.NewProc("GlobalAlloc")
	globalLock := kernel32.NewProc("GlobalLock")
	globalUnlock := kernel32.NewProc("GlobalUnlock")

	const cfUnicode = 13
	const gmemMoveable = 0x0002

	r, _, _ := open.Call(0)
	if r == 0 {
		return errControlInvalid
	}
	defer closeClip.Call()

	empty.Call()
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := len(utf16) * 2
	h, _, _ := globalAlloc.Call(gmemMoveable, uintptr(size))
	if h == 0 {
		return errControlInvalid
	}
	ptr, _, _ := globalLock.Call(h)
	if ptr == 0 {
		return errControlInvalid
	}
	copyMem := (*[1 << 20]uint16)(unsafe.Pointer(ptr))[:len(utf16):len(utf16)]
	copy(copyMem, utf16)
	globalUnlock.Call(h)
	setData.Call(cfUnicode, h)
	return nil
}

func controlWinD() error {
	const vkLWin = 0x5B
	const vkD = 0x44
	sendKey(vkLWin, false)
	sendKey(vkD, false)
	sendKey(vkD, true)
	sendKey(vkLWin, true)
	return nil
}

type fileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func controlListDownloads() ([]fileEntry, error) {
	dir, err := connectTransferDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, fileEntry{Name: e.Name(), Size: info.Size()})
	}
	return out, nil
}
