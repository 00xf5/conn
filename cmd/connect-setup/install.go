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

	progress("Downloading", "Getting BlueConnect…")
	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("blueconnect-setup-%d.zip", time.Now().UnixNano()))
	defer os.Remove(zipPath)
	if err := downloadFile(agentURL, zipPath); err != nil {
		return fmt.Errorf("download failed — check your network or ask your tech to publish the agent package: %w", err)
	}

	progress("Preparing", "Stopping any previous agent…")
	stopExistingAgent()

	progress("Installing", "Setting up BlueConnect on this PC…")
	if err := unzipTo(zipPath, dest); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	exe := filepath.Join(dest, "connect-agent.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("install incomplete — agent missing. Ask your tech to re-publish the package")
	}

	if enrolled() {
		progress("Enrolling", "Already enrolled on this PC…")
	} else {
		progress("Enrolling", "Linking this PC…")
		cmd := exec.Command(exe, "-server", server, "-enroll", code, "-quit-after-enroll")
		cmd.Dir = dest
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

	progress("Starting", "Updating Windows service…")
	if err := runElevatedInstallService(exe, dest); err != nil {
		progress("Starting", "Starting agent…")
		start := exec.Command(exe)
		start.Dir = dest
		_ = start.Start()
	}

	progress("Done", "This PC is ready. You can close this window.")
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "BlueConnect-Setup")
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
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func stopExistingAgent() {
	_ = exec.Command("sc.exe", "stop", "ConnectAgent").Run()
	_ = exec.Command("taskkill.exe", "/IM", "connect-agent.exe", "/F").Run()
	// Wait until the service is stopped / exe unlocked so we can overwrite files.
	for i := 0; i < 40; i++ {
		out, _ := exec.Command("sc.exe", "query", "ConnectAgent").CombinedOutput()
		s := string(out)
		if !strings.Contains(s, "RUNNING") && !strings.Contains(s, "STOP_PENDING") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
}

func runElevatedInstallService(exe, dir string) error {
	// Prefer direct call first (already admin).
	cmd := exec.Command(exe, "-install-service")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		return nil
	}
	// Elevate via PowerShell Start-Process -Verb RunAs (stop+reinstall handled inside -install-service).
	ps := fmt.Sprintf(
		"$p=Start-Process -FilePath %s -WorkingDirectory %s -Verb RunAs -Wait -PassThru -ArgumentList @('-install-service'); if($null -eq $p){exit 1}; if($null -eq $p.ExitCode){exit 0}; exit [int]$p.ExitCode",
		powershellQuote(exe),
		powershellQuote(dir),
	)
	elev := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	return elev.Run()
}

func powershellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
