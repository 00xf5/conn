//go:build ignore

// Embeds icon.ico into rsrc_windows_amd64.syso for the Go linker.
// Run from repo root: go run ./cmd/connect-host/genrsrc.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// Allow running from cmd/connect-host.
	ico := filepath.Join(root, "cmd", "connect-host", "icon.ico")
	out := filepath.Join(root, "cmd", "connect-host", "rsrc_windows_amd64.syso")
	if _, err := os.Stat(ico); err != nil {
		ico = filepath.Join(root, "icon.ico")
		out = filepath.Join(root, "rsrc_windows_amd64.syso")
	}
	if _, err := os.Stat(ico); err != nil {
		panic("icon.ico missing — run genicon.go first")
	}

	cmd := exec.Command("go", "run", "github.com/akavel/rsrc@v0.10.2",
		"-arch", "amd64",
		"-ico", ico,
		"-o", out,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
	fmt.Println("wrote", out)
}
