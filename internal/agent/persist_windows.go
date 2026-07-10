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

	// Clear any leftover visible tasklist/watchdog loops from older builds.
	stopWatchdogProcesses()

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
		path := filepath.Join(startupDir, name)
		if err := os.Remove(path); err == nil {
			log.Printf("agent: removed legacy Startup %q (service owns persistence)", name)
		}
	}
	// Drop leftover watchdog scripts next to the agent when the service is installed.
	if dir := DataDir(); dir != "" {
		for _, name := range []string{watchdogName, watchdogVbs} {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	stopWatchdogProcesses()
}

func buildWatchdogCmd(dir, exe string) string {
	// No tasklist.exe — it flashes a console even when redirected.
	// PowerShell -WindowStyle Hidden checks for the agent process quietly.
	ps := `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe`
	return fmt.Sprintf("@echo off\r\n"+
		"title %s\r\n"+
		"cd /d \"%s\"\r\n"+
		":loop\r\n"+
		"\"%s\" -NoProfile -NonInteractive -WindowStyle Hidden -Command "+
		"\"if (-not (Get-Process -Name connect-agent -ErrorAction SilentlyContinue)) { exit 1 } else { exit 0 }\" >nul 2>&1\r\n"+
		"if errorlevel 1 (\r\n"+
		"  start /wait \"\" \"%s\"\r\n"+
		")\r\n"+
		"timeout /t 5 /nobreak >nul\r\n"+
		"goto loop\r\n", watchTitle, dir, ps, exe)
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
	hideConsole(cmd)
	return cmd.Start()
}

func watchdogRunning() bool {
	// Prefer title match without a visible console (CREATE_NO_WINDOW).
	cmd := exec.Command("tasklist", "/V", "/FO", "CSV", "/NH")
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), watchTitle)
}

func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}

func stopWatchdogProcesses() {
	// Kill leftover visible/hidden watchdog consoles left from older installs.
	ps := `$p = Get-CimInstance Win32_Process -Filter "Name='cmd.exe'" | Where-Object { $_.CommandLine -match 'Connect-Watch|Connect Agent Watchdog' }; foreach ($x in $p) { Stop-Process -Id $x.ProcessId -Force -ErrorAction SilentlyContinue }; Get-Process | Where-Object { $_.MainWindowTitle -eq 'Connect Agent Watchdog' } | Stop-Process -Force -ErrorAction SilentlyContinue`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", ps)
	hideConsole(cmd)
	_ = cmd.Run()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
