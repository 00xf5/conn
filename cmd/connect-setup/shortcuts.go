package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// createHostShortcuts places Desktop + Start Menu shortcuts to WorthyJoin-Host.exe.
// Failures are non-fatal — agent install still succeeds.
func createHostShortcuts(dest string) error {
	hostExe := filepath.Join(dest, "WorthyJoin-Host.exe")
	if _, err := os.Stat(hostExe); err != nil {
		return fmt.Errorf("WorthyJoin-Host.exe missing — shortcuts skipped")
	}

	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", "WorthyJoin Host.lnk")
	startMenu := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "WorthyJoin Host.lnk")

	for _, lnk := range []string{desktop, startMenu} {
		if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
			return err
		}
		if err := writeShortcut(lnk, hostExe, dest); err != nil {
			return err
		}
	}
	return nil
}

func writeShortcut(lnkPath, target, workDir string) error {
	ps := fmt.Sprintf(
		`$ws=New-Object -ComObject WScript.Shell; $s=$ws.CreateShortcut(%s); $s.TargetPath=%s; $s.WorkingDirectory=%s; $s.IconLocation=%s; $s.Description='WorthyJoin Host'; $s.Save()`,
		powershellQuote(lnkPath),
		powershellQuote(target),
		powershellQuote(workDir),
		powershellQuote(target+",0"),
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-Command", ps)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("shortcut %s: %v (%s)", lnkPath, err, strings.TrimSpace(string(out)))
	}
	return nil
}
