//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var procCreateMutexW = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW")

// acquireSingleInstance returns false if another connect-agent is already running.
func acquireSingleInstance() (release func(), ok bool) {
	name, _ := syscall.UTF16PtrFromString("Global\\ConnectHostAgent")
	handle, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, false
	}
	if err == syscall.ERROR_ALREADY_EXISTS {
		syscall.CloseHandle(syscall.Handle(handle))
		return nil, false
	}
	return func() { syscall.CloseHandle(syscall.Handle(handle)) }, true
}

func exitIfAlreadyRunning() {
	if _, ok := acquireSingleInstance(); ok {
		return
	}
	fmt.Fprintln(os.Stderr, "connect-agent already running (check system tray)")
	os.Exit(0)
}
