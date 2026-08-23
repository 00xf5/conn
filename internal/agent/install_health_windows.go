//go:build windows

package agent

import (
	"os"
	"path/filepath"
)

// installHealthWarn returns a short dashboard-visible warning when the install tree is incomplete.
func installHealthWarn() string {
	exe, err := os.Executable()
	if err != nil {
		return "agent path unknown"
	}
	dir := filepath.Dir(exe)
	agentExe := filepath.Join(dir, "connect-agent.exe")
	if st, err := os.Stat(agentExe); err != nil || st.IsDir() {
		// Service may run from connect-agent.exe directly.
		if _, err := os.Stat(exe); err != nil {
			return "connect-agent.exe missing"
		}
	}
	ff := filepath.Join(dir, "bin", "ffmpeg.exe")
	if st, err := os.Stat(ff); err != nil || st.IsDir() {
		return "missing bin\\ffmpeg.exe — reinstall from dashboard Setup"
	}
	return ""
}
