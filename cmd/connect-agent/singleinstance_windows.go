//go:build windows

package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

var (
	procCreateMutexW = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW")
	instanceMu       sync.Mutex
	instanceHandle   syscall.Handle
)

// acquireSingleInstance returns false if another connect-agent is already running.
// On success the mutex handle is held for the process lifetime (do not close early).
func acquireSingleInstance() bool {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	if instanceHandle != 0 {
		return true
	}
	name, err := syscall.UTF16PtrFromString("Global\\ConnectHostAgent")
	if err != nil {
		return true // fail open — better one agent than none
	}
	handle, _, callErr := procCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return true
	}
	h := syscall.Handle(handle)
	if callErr == syscall.ERROR_ALREADY_EXISTS {
		_ = syscall.CloseHandle(h)
		return false
	}
	instanceHandle = h
	return true
}

func exitIfAlreadyRunning() {
	if acquireSingleInstance() {
		return
	}
	fmt.Fprintln(os.Stderr, "connect-agent already running (check system tray)")
	os.Exit(0)
}
