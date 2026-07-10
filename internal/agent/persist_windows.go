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
	startupName    = "Connect Agent.vbs"
	startupNameOld = "Connect Agent.cmd"
	watchdogName   = "Connect-Watch.cmd"
	watchdogVbs    = "Connect-Watch.vbs"
	watchTitle     = "Connect Agent Watchdog"
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
	vbsPath := filepath.Join(dir, watchdogVbs)
	if err := writeIfChanged(watchPath, buildWatchdogCmd(dir, exe)); err != nil {
		log.Printf("agent: persistence watchdog: %v", err)
		return
	}
	if err := writeIfChanged(vbsPath, buildWatchdogVBS(watchPath)); err != nil {
		log.Printf("agent: persistence watchdog vbs: %v", err)
		return
	}

	startupDir := filepath.Join(os.Getenv("APPDATA"),
		`Microsoft\Windows\Start Menu\Programs\Startup`)
	if err := os.MkdirAll(startupDir, 0o755); err != nil {
		log.Printf("agent: persistence startup dir: %v", err)
		return
	}
	// Remove old visible .cmd Startup entry if present.
	_ = os.Remove(filepath.Join(startupDir, startupNameOld))

	startupPath := filepath.Join(startupDir, startupName)
	if err := writeIfChanged(startupPath, buildStartupVBS(vbsPath)); err != nil {
		log.Printf("agent: persistence startup: %v", err)
		return
	}

	if watchdogRunning() {
		log.Printf("agent: persistence ok (watchdog already running)")
		return
	}
	if err := startWatchdogHidden(vbsPath); err != nil {
		log.Printf("agent: persistence start watchdog: %v", err)
		return
	}
	log.Printf("agent: persistence ok (hidden watchdog + Startup\\%s)", startupName)
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
	startupDir := filepath.Join(os.Getenv("APPDATA"),
		`Microsoft\Windows\Start Menu\Programs\Startup`)
	for _, name := range []string{startupName, startupNameOld} {
		_ = os.Remove(filepath.Join(startupDir, name))
	}
}

func buildWatchdogCmd(dir, exe string) string {
	// Runs under a hidden console (launched via VBS window style 0).
	// start /wait keeps the loop blocked on the GUI agent without a second window.
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

func buildWatchdogVBS(watchCmdPath string) string {
	// WindowStyle 0 = hidden — no taskbar console after reboot.
	return fmt.Sprintf(
		"Set sh = CreateObject(\"WScript.Shell\")\r\n"+
			"sh.Run \"cmd.exe /c \"\"%s\"\"\", 0, False\r\n",
		escapeVBS(watchCmdPath),
	)
}

func buildStartupVBS(watchVBSPath string) string {
	return fmt.Sprintf(
		"CreateObject(\"WScript.Shell\").Run \"wscript.exe //B //Nologo \"\"%s\"\"\", 0, False\r\n",
		escapeVBS(watchVBSPath),
	)
}

func escapeVBS(path string) string {
	return strings.ReplaceAll(path, `"`, `""`)
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

func startWatchdogHidden(vbsPath string) error {
	cmd := exec.Command("wscript.exe", "//B", "//Nologo", vbsPath)
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
