//go:build windows

package agent

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func prepareHiddenCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
