//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
