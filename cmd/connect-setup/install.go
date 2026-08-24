package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type InstallOptions struct {
	Code     string
	Server   string // wss://host/ws
	AgentURL string // https://host/download/agent.zip
	Quiet    bool
}

type ProgressFunc func(step, detail string)

func defaultServer() string {
	return "wss://worthyjoin.online/ws"
}

func defaultAgentURL() string {
	return "https://worthyjoin.online/download/agent.zip"
}

func normalizeServer(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return defaultServer()
	}
	s = strings.TrimRight(s, "/")
	switch {
	case strings.HasPrefix(s, "https://"):
		s = "wss://" + strings.TrimPrefix(s, "https://")
	case strings.HasPrefix(s, "http://"):
		s = "ws://" + strings.TrimPrefix(s, "http://")
	case !strings.HasPrefix(s, "wss://") && !strings.HasPrefix(s, "ws://"):
		s = "wss://" + s
	}
	if !strings.HasSuffix(s, "/ws") {
		s += "/ws"
	}
	return s
}

func agentURLFromServer(server string) string {
	u := strings.TrimSpace(server)
	u = strings.Replace(u, "wss://", "https://", 1)
	u = strings.Replace(u, "ws://", "http://", 1)
	u = strings.TrimSuffix(u, "/ws")
	u = strings.TrimRight(u, "/")
	return u + "/download/agent.zip"
}

func connectDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Connect")
}

func enrolled() bool {
	cfg := filepath.Join(connectDir(), "config.json")
	b, err := os.ReadFile(cfg)
	if err != nil {
		return false
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	tid, _ := m["tenantId"].(string)
	return strings.TrimSpace(tid) != ""
}

// localAgentZip returns agent.zip next to Setup (from WorthyJoin-Install.zip extract).
func localAgentZip() string {
	var dirs []string
	if exe, err := os.Executable(); err == nil && exe != "" {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		dirs = append(dirs, cwd)
	}
	names := []string{"agent.zip", "WorthyJoin-agent.zip"}
	seen := map[string]bool{}
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		for _, name := range names {
			p := filepath.Join(dir, name)
			st, err := os.Stat(p)
			if err == nil && !st.IsDir() && st.Size() > 1000 {
				return p
			}
		}
	}
	return ""
}

func runInstall(opts InstallOptions, progress ProgressFunc) error {
	if progress == nil {
		progress = func(string, string) {}
	}
	code := strings.TrimSpace(opts.Code)
	if code == "" {
		return fmt.Errorf("enrollment code is required")
	}
	server := normalizeServer(opts.Server)
	agentURL := strings.TrimSpace(opts.AgentURL)
	if agentURL == "" {
		agentURL = agentURLFromServer(server)
	}

	dest := connectDir()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create install folder: %w", err)
	}

	// Clear Mark-of-the-Web from the extracted install bundle (zip download).
	unblockSetupBundle()

	var zipPath string
	if local := localAgentZip(); local != "" {
		progress("Installing", "Using files from your download folder…")
		zipPath = local
		unblockPath(local)
	} else {
		progress("Downloading", "Getting WorthyJoin…")
		zipPath = filepath.Join(os.TempDir(), fmt.Sprintf("worthyjoin-setup-%d.zip", time.Now().UnixNano()))
		defer os.Remove(zipPath)
		if err := downloadFile(agentURL, zipPath); err != nil {
			return fmt.Errorf("download failed — check your network or ask your tech to publish the agent package: %w", err)
		}
	}

	progress("Preparing", "Stopping any previous agent/service…")
	writeInstallPause(dest)
	defer clearInstallPause(dest)
	time.Sleep(2200 * time.Millisecond) // one supervisor tick so it will not relaunch
	stopExistingAgent()
	if serviceStillRunning() {
		progress("Preparing", "Windows may ask for permission to stop the old agent…")
		stopServiceElevated()
		stopExistingAgent()
	}

	progress("Installing", "Updating WorthyJoin files (enrollment kept)…")
	if err := unzipTo(zipPath, dest); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}
	unblockInstallTree(dest)

	exe := filepath.Join(dest, "connect-agent.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("install incomplete — agent missing. Ask your tech to re-publish the package")
	}

	// Preserve existing enrollment (config.json is not in the zip). Only redeem a new code if needed.
	if enrolled() {
		progress("Enrolling", "Already enrolled — keeping this PC’s identity…")
	} else {
		progress("Enrolling", "Linking this PC…")
		cmd := exec.Command(exe, "-server", server, "-enroll", code, "-quit-after-enroll")
		cmd.Dir = dest
		hideConsole(cmd)
		out, err := cmd.CombinedOutput()
		if !enrolled() {
			msg := strings.TrimSpace(string(out))
			if msg == "" && err != nil {
				msg = err.Error()
			}
			if msg == "" {
				msg = "enrollment failed — ask for a fresh install link"
			}
			return fmt.Errorf("%s", msg)
		}
	}

	progress("Shortcuts", "Adding Desktop shortcut…")
	if err := createHostShortcuts(dest); err != nil {
		progress("Shortcuts", "Shortcut skipped — Host app is in the install folder")
	}

	// Bring the agent online immediately so the tech dashboard updates even if UAC is slow.
	progress("Starting", "Connecting this PC…")
	startAgentDetached(exe, dest)

	// Don't block the installer on the UAC prompt — machine is already linking.
	progress("Permission", "If Windows asks for permission, click Yes (keeps the agent running after reboot)…")
	go func() { _ = runElevatedInstallService(exe, dest) }()
	time.Sleep(800 * time.Millisecond)

	progress("Done", "This PC is ready. You can close this window.")
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "WorthyJoin-Setup")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", res.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func unzipTo(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(dest, name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			continue
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		writeErr := writeFileReplace(rc, target, f.Mode())
		_ = rc.Close()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// writeFileReplace stages src to a temp file, then replaces target. Staging is
// required so retries do not re-read an already-consumed zip stream.
func writeFileReplace(src io.Reader, target string, mode os.FileMode) error {
	stage, err := os.CreateTemp(filepath.Dir(target), ".wj-write-*")
	if err != nil {
		stage, err = os.CreateTemp("", ".wj-write-*")
		if err != nil {
			return err
		}
	}
	stagePath := stage.Name()
	_, copyErr := io.Copy(stage, src)
	closeErr := stage.Close()
	defer os.Remove(stagePath)
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}

	var last error
	for attempt := 0; attempt < 12; attempt++ {
		if attempt > 0 {
			stopExistingAgent()
			if attempt == 2 && serviceStillRunning() {
				stopServiceElevated()
				stopExistingAgent()
			}
			time.Sleep(400 * time.Millisecond)
		}
		in, err := os.Open(stagePath)
		if err != nil {
			return err
		}
		err = tryWriteFile(in, target, mode)
		_ = in.Close()
		if err == nil {
			return nil
		}
		last = err

		// Windows often allows renaming a locked executable (service still mapped).
		old := target + ".old"
		_ = os.Remove(old)
		if err := os.Rename(target, old); err != nil {
			last = err
			continue
		}
		in, err = os.Open(stagePath)
		if err != nil {
			_ = os.Rename(old, target)
			return err
		}
		err = tryWriteFile(in, target, mode)
		_ = in.Close()
		if err != nil {
			_ = os.Rename(old, target)
			last = err
			continue
		}
		_ = os.Remove(old)
		return nil
	}
	return fmt.Errorf("open %s: file in use — close WorthyJoin / ConnectAgent and try again (%v)", target, last)
}

func tryWriteFile(src io.Reader, target string, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func installPausePath(dir string) string {
	return filepath.Join(dir, ".worthyjoin-updating")
}

func writeInstallPause(dir string) {
	_ = os.WriteFile(installPausePath(dir), []byte("1"), 0o644)
}

func clearInstallPause(dir string) {
	_ = os.Remove(installPausePath(dir))
}

func serviceStillRunning() bool {
	s := string(combinedHidden("sc.exe", "query", "ConnectAgent"))
	return strings.Contains(s, "RUNNING") || strings.Contains(s, "START_PENDING") || strings.Contains(s, "STOP_PENDING")
}

var elevatedStopAttempted bool

func stopServiceElevated() {
	if elevatedStopAttempted {
		return
	}
	elevatedStopAttempted = true
	// Friend PCs: ConnectAgent runs as SYSTEM from the same exe. Unelevated
	// sc stop / taskkill cannot release that lock. One UAC prompt is enough.
	ps := `$ErrorActionPreference='SilentlyContinue'; ` +
		`Start-Process -FilePath 'sc.exe' -ArgumentList @('stop','ConnectAgent') -Verb RunAs -WindowStyle Hidden -Wait; ` +
		`Start-Sleep -Milliseconds 400; ` +
		`Get-Process connect-agent,WorthyJoin-Host -ErrorAction SilentlyContinue | Stop-Process -Force`
	elev := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-Command", ps)
	hideConsole(elev)
	_ = elev.Run()
	time.Sleep(800 * time.Millisecond)
}

func stopExistingAgent() {
	writeInstallPause(connectDir())
	runHidden("sc.exe", "stop", "ConnectAgent")

	for i := 0; i < 40; i++ {
		if !serviceStillRunning() {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if serviceStillRunning() {
		runHidden("net.exe", "stop", "ConnectAgent", "/y")
		time.Sleep(400 * time.Millisecond)
	}

	runHidden("taskkill.exe", "/F", "/IM", "connect-agent.exe")
	runHidden("taskkill.exe", "/F", "/IM", "WorthyJoin-Host.exe")
	time.Sleep(400 * time.Millisecond)
}

func runHidden(name string, args ...string) {
	cmd := exec.Command(name, args...)
	hideConsole(cmd)
	_ = cmd.Run()
}

func combinedHidden(name string, args ...string) []byte {
	cmd := exec.Command(name, args...)
	hideConsole(cmd)
	out, _ := cmd.CombinedOutput()
	return out
}

func runElevatedInstallService(exe, dir string) error {
	cmd := exec.Command(exe, "-install-service")
	cmd.Dir = dir
	hideConsole(cmd)
	if err := cmd.Run(); err == nil {
		return nil
	}
	ps := fmt.Sprintf(
		"$p=Start-Process -FilePath %s -WorkingDirectory %s -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList @('-install-service'); if($null -eq $p){exit 1}; if($null -eq $p.ExitCode){exit 0}; exit [int]$p.ExitCode",
		powershellQuote(exe),
		powershellQuote(dir),
	)
	elev := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-Command", ps)
	hideConsole(elev)
	return elev.Run()
}

func startAgentDetached(exe, dir string) {
	start := exec.Command(exe)
	start.Dir = dir
	setDetached(start)
	_ = start.Start()
	if start.Process != nil {
		_ = start.Process.Release()
	}
}

func powershellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
