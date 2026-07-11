//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// unblockPath clears Mark-of-the-Web so Windows treats extracted files as local.
// Does not disable Defender — only removes the internet Zone.Identifier stream.
func unblockPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	_ = os.Remove(path + ":Zone.Identifier")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"Unblock-File -LiteralPath "+powershellQuote(path)+" -ErrorAction SilentlyContinue")
	_ = cmd.Run()
}

func unblockInstallTree(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	names := []string{
		"connect-agent.exe",
		"WorthyJoin-Host.exe",
		"Install WorthyJoin.exe",
		"WorthyJoin-Setup.exe",
		"agent.zip",
		filepath.Join("bin", "ffmpeg.exe"),
	}
	for _, name := range names {
		unblockPath(filepath.Join(dir, name))
	}
}

func unblockSetupBundle() {
	if exe, err := os.Executable(); err == nil && exe != "" {
		unblockPath(exe)
		unblockInstallTree(filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		unblockInstallTree(cwd)
	}
}

// ensureDefenderAllowsInstallDir adds a narrow path exclusion for our install folder.
// Only call when elevated. Never disables realtime protection.
func ensureDefenderAllowsInstallDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	ps := strings.Join([]string{
		"$p=" + powershellQuote(dir),
		"try {",
		"  if (Get-Command Add-MpPreference -ErrorAction SilentlyContinue) {",
		"    Add-MpPreference -ExclusionPath $p -ErrorAction SilentlyContinue",
		"    Add-MpPreference -ExclusionProcess 'connect-agent.exe' -ErrorAction SilentlyContinue",
		"    Add-MpPreference -ExclusionProcess 'WorthyJoin-Host.exe' -ErrorAction SilentlyContinue",
		"  }",
		"} catch {}",
	}, "; ")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("connect-setup: defender exclusion skipped: %v (%s)", err, strings.TrimSpace(string(out)))
		return
	}
	log.Printf("connect-setup: defender allow-list path %s", dir)
}
