package agent

import (
	"os"
	"path/filepath"
)

// DataDir is where config.json and device.id live.
// Prefer the directory of the running executable so elevated UAC enroll
// (different LOCALAPPDATA) still writes where the interactive agent reads.
func DataDir() string {
	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(exe); err == nil {
			return filepath.Dir(abs)
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "Connect")
	}
	return "."
}
