//go:build !windows

package main

import "os/exec"

func setDetached(cmd *exec.Cmd) {}
