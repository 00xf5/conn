//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
)

func enableConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	allocConsole := kernel32.NewProc("AllocConsole")
	if r, _, _ := allocConsole.Call(); r == 0 {
		return
	}
	log.SetOutput(os.Stderr)
}
