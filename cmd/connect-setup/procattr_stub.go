//go:build !windows

package main

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}

func setDetached(cmd *exec.Cmd) {}
