//go:build windows

package agent

import "syscall"

var procBlockInput = user32.NewProc("BlockInput")

func setBlockInput(block bool) error {
	var arg uintptr
	if block {
		arg = 1
	}
	r, _, err := procBlockInput.Call(arg)
	if r == 0 {
		if err != nil && err != syscall.Errno(0) {
			return errString("block local input failed: " + err.Error())
		}
		return errString("block local input failed")
	}
	return nil
}
