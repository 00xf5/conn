//go:build windows

package agent

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/svc/mgr"
)

const (
	startupName   = "Connect Agent.cmd"
	watchdogName  = "Connect-Watch.cmd"
	watchTitle    = "Connect Agent Watchdog"
	windowsSvcName = "ConnectAgent"
)

// EnsurePersistence installs fallback Startup/watchdog persistence when the
// Windows Service is not installed. If ConnectAgent service exists, the SCM
// supervisor owns reboot/crash recovery — do not double-supervise.
func EnsurePersistence() {
	if windowsServiceInstalled() {
		removeLegacyStartup()
		log.Printf("agent: persistence via Windows Service %q", windowsSvcName)
		return
	}

	exe, err := os.Executable()
	if err != nil || exe == "" {
		return
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return
	}

	dir := filepath.Dir(exe)
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		preferred := filepath.Join(local, "Connect")
		prefExe := filepath.Join(preferred, "connect-agent.exe")
		if fileExists(prefExe) {
			dir = preferred
			exe = prefExe
		}
	}

	watchPath := filepath.Join(dir, watchdogName)
	if err := writeIfChanged(watchPath, buildWatchdogCmd(dir, exe)); err != nil {
		log.Printf("agent: persistence watchdog: %v", err)
		return
	}

	startupDir := filepath.Join(os.Getenv("APPDATA"),
		`Microsoft\Windows\Start Menu\Programs\Startup`)
	if err := os.MkdirAll(startupDir, 0o755); err != nil {
		log.Printf("agent: persistence startup dir: %v", err)
		return
	}
	startupPath := filepath.Join(startupDir, startupName)
	startBody := fmt.Sprintf("@echo off\r\n"+
		"rem Connect fallback — used only when Windows Service is not installed\r\n"+
		"start \"%s\" /min \"%s\"\r\n", watchTitle, watchPath)
	if err := writeIfChanged(startupPath, startBody); err != nil {
		log.Printf("agent: persistence startup: %v", err)
		return
	}

	if watchdogRunning() {
		log.Printf("agent: persistence ok (watchdog already running)")
		return
	}
	if err := startWatchdog(watchPath); err != nil {
		log.Printf("agent: persistence start watchdog: %v", err)
		return
	}
	log.Printf("agent: persistence ok (watchdog + Startup\\%s)", startupName)
}

func windowsServiceInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(windowsSvcName)
	if err != nil {
		return false
	}
	_ = s.Close()
	return true
}

func removeLegacyStartup() {
	startup := filepath.Join(os.Getenv("APPDATA"),
		`Microsoft\Windows\Start Menu\Programs\Startup`, startupName)
	_ = os.Remove(startup)
}

func buildWatchdogCmd(dir, exe string) string {
	return fmt.Sprintf("@echo off\r\n"+
		"title %s\r\n"+
		"cd /d \"%s\"\r\n"+
		":loop\r\n"+
		"tasklist /FI \"IMAGENAME eq connect-agent.exe\" 2>nul | find /I \"connect-agent.exe\" >nul\r\n"+
		"if errorlevel 1 (\r\n"+
		"  start /wait \"\" \"%s\"\r\n"+
		")\r\n"+
		"timeout /t 5 /nobreak >nul\r\n"+
		"goto loop\r\n", watchTitle, dir, exe)
}

func writeIfChanged(path, body string) error {
	if b, err := os.ReadFile(path); err == nil && string(b) == body {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func startWatchdog(watchPath string) error {
	cmd := exec.Command("cmd.exe", "/C", "start", watchTitle, "/min", watchPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func watchdogRunning() bool {
	out, err := exec.Command("tasklist", "/V", "/FO", "CSV", "/NH").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), watchTitle)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
