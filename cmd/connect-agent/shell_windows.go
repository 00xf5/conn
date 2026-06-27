//go:build windows

package main

import "os/exec"

func shellOpen(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Run()
}
