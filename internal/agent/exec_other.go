//go:build !windows

package agent

import "os/exec"

func prepareHiddenCmd(cmd *exec.Cmd) {}
