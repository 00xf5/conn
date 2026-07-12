//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow   = 0x08000000
	detachedProcess  = 0x00000008
)

// hideConsole prevents console subsystem tools (powershell, sc, taskkill, …)
// from flashing a visible terminal during install.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNoWindow,
	}
}
