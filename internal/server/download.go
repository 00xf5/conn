package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) agentDir() string {
	if d := strings.TrimSpace(s.cfg.AgentDir); d != "" {
		return d
	}
	return "data/agent"
}

func (s *Server) agentZipPath() string {
	return filepath.Join(s.agentDir(), "agent.zip")
}

func (s *Server) agentPackageAvailable() bool {
	st, err := os.Stat(s.agentZipPath())
	return err == nil && st.Size() > 0
}

func (s *Server) handleAgentPackageInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.agentZipPath()
	st, err := os.Stat(path)
	if err != nil {
		writeJSON(w, map[string]any{
			"available": false,
			"agentDir":  s.agentDir(),
			"hint":      "Upload agent.zip in Admin → Agent package (or publish-agent.ps1)",
		})
		return
	}
	writeJSON(w, map[string]any{
		"available": true,
		"size":      st.Size(),
		"updatedAt": st.ModTime().UTC().Format(time.RFC3339),
		"download":  "/download/agent.zip",
		"install":   "/install",
	})
}

const maxAgentUploadBytes = 200 << 20 // 200 MiB

// Admin uploads agent.zip (no SSH/SCP required).
func (s *Server) handleAdminAgentPackage(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleAgentPackageInfo(w, r)
	case http.MethodPost:
		s.handleAdminAgentPackageUpload(w, r)
	case http.MethodDelete:
		path := s.agentZipPath()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "available": false})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminAgentPackageUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentUploadBytes)
	if err := r.ParseMultipartForm(maxAgentUploadBytes); err != nil {
		http.Error(w, "upload too large or invalid (max 200MB)", http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required (multipart form field name: file)", http.StatusBadRequest)
		return
	}
	defer file.Close()

	name := strings.ToLower(filepath.Base(hdr.Filename))
	if !strings.HasSuffix(name, ".zip") {
		http.Error(w, "file must be a .zip (agent.zip)", http.StatusBadRequest)
		return
	}

	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		http.Error(w, "empty or unreadable upload", http.StatusBadRequest)
		return
	}
	if magic[0] != 'P' || magic[1] != 'K' {
		http.Error(w, "not a zip file", http.StatusBadRequest)
		return
	}

	dir := s.agentDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmp := filepath.Join(dir, "agent.zip.uploading")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := out.Write(magic[:]); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nRest, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	n := int64(len(magic)) + nRest
	if copyErr != nil {
		_ = os.Remove(tmp)
		http.Error(w, copyErr.Error(), http.StatusInternalServerError)
		return
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		return
	}
	if n < 1000 {
		_ = os.Remove(tmp)
		http.Error(w, "upload too small", http.StatusBadRequest)
		return
	}
	final := s.agentZipPath()
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok":        true,
		"available": true,
		"size":      n,
		"path":      final,
	})
}

func (s *Server) handleDownloadAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.agentZipPath()
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "agent package not published yet — run deploy/publish-agent.ps1", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="connect-agent.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func (s *Server) publicBase(r *http.Request) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.PublicURL), "/")
	if base != "" && !strings.Contains(base, "localhost") && !strings.Contains(base, "127.0.0.1") {
		return base
	}
	scheme := "https"
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8787"
	}
	return scheme + "://" + host
}

func (s *Server) installURL(r *http.Request, code string) string {
	u := s.publicBase(r) + "/install"
	if code != "" {
		u += "?code=" + strings.TrimSpace(code)
	}
	return u
}

func (s *Server) handleInstallPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	base := s.publicBase(r)
	available := s.agentPackageAvailable()

	status := `<p class="ok">Agent package ready — download below.</p>`
	if !available {
		status = `<p class="warn">Agent package is not on the server yet. Ask your admin to upload it (Admin → Agent package).</p>`
	}

	codeBlock := ""
	setupBtn := ""
	if code != "" {
		codeBlock = fmt.Sprintf(`<p class="code-label">Enrollment code</p><code class="big">%s</code>`, htmlEscape(code))
		setupHref := htmlEscape(base + "/download/setup.cmd?code=" + code)
		if available {
			setupBtn = fmt.Sprintf(`<a class="btn" href="%s">Download installer (Connect-Install.cmd)</a>`, setupHref)
		} else {
			setupBtn = `<span class="btn muted-btn">Download installer (package missing)</span>`
		}
	} else {
		codeBlock = `<p class="muted">Open this page from an enrollment link your tech sent.</p>`
	}

	zipHref := htmlEscape(base + "/download/agent.zip")
	zipBtn := ""
	if available {
		zipBtn = fmt.Sprintf(`<a class="btn secondary" href="%s">Download agent only (.zip)</a>`, zipHref)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Install Connect</title>
<style>
body{font:15px/1.45 system-ui,Segoe UI,sans-serif;margin:0;background:#e8eaee;color:#1f2430}
.wrap{max-width:520px;margin:48px auto;padding:0 16px}
.card{background:#fff;border:1px solid #d0d4dc;border-radius:6px;padding:22px;display:grid;gap:12px}
h1{margin:0;font-size:22px}
.muted{color:#6b7280;margin:0}
.ok{color:#1a9f4b;margin:0}
.warn{color:#b45309;margin:0;background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:4px}
.code-label{margin:0;font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.04em;color:#6b7280}
.big{font:700 16px Consolas,monospace;word-break:break-all;background:#f4f5f7;padding:10px;border-radius:4px;display:block}
.steps{margin:0;padding-left:18px}
.steps li{margin:8px 0}
.btn{display:inline-block;text-align:center;text-decoration:none;border:0;background:#0b5fff;color:#fff;font:inherit;font-weight:700;padding:12px 14px;border-radius:4px}
.btn:hover{background:#094fd6}
.btn.secondary{background:#fff;color:#1f2430;border:1px solid #b8bec9}
.btn.secondary:hover{background:#f3f5f8}
.muted-btn{background:#9aa1ad;pointer-events:none;display:inline-block;padding:12px 14px;border-radius:4px;color:#fff;font-weight:700}
.actions{display:grid;gap:10px}
</style>
</head>
<body>
<div class="wrap"><div class="card">
<h1>Install Connect</h1>
<p class="muted">Normal download + double-click install on this Windows PC.</p>
%s
%s
<div class="actions">
%s
%s
</div>
<ol class="steps">
<li>Click <strong>Download installer</strong>.</li>
<li>Double-click <strong>Connect-Install.cmd</strong> (downloads the agent, enrolls, starts).</li>
<li>This PC appears in the Host console when online.</li>
</ol>
<p class="muted">Windows only.</p>
</div></div>
</body></html>`, status, codeBlock, setupBtn, zipBtn)
}

func (s *Server) handleDownloadSetupCmd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Error(w, "code required", http.StatusBadRequest)
		return
	}
	base := s.publicBase(r)
	wss := strings.Replace(base, "https://", "wss://", 1)
	wss = strings.Replace(wss, "http://", "ws://", 1)
	if !strings.HasSuffix(wss, "/ws") {
		wss = strings.TrimRight(wss, "/") + "/ws"
	}
	// Escape for cmd.exe: double quotes in paths; keep code simple.
	zipURL := base + "/download/agent.zip"
	body := fmt.Sprintf("@echo off\r\n"+
		"setlocal\r\n"+
		"title Connect Install\r\n"+
		"echo Connect installer\r\n"+
		"echo.\r\n"+
		"set \"DEST=%%LOCALAPPDATA%%\\Connect\"\r\n"+
		"set \"ZIP=%%TEMP%%\\connect-agent.zip\"\r\n"+
		"set \"SERVER=%s\"\r\n"+
		"set \"CODE=%s\"\r\n"+
		"if not exist \"%%DEST%%\" mkdir \"%%DEST%%\"\r\n"+
		"echo Downloading agent...\r\n"+
		"curl.exe -fsSL \"%s\" -o \"%%ZIP%%\"\r\n"+
		"if errorlevel 1 (\r\n"+
		"  echo Download failed. Check network / agent package on server.\r\n"+
		"  pause\r\n"+
		"  exit /b 1\r\n"+
		")\r\n"+
		"taskkill /IM connect-agent.exe /F >nul 2>&1\r\n"+
		"timeout /t 1 /nobreak >nul\r\n"+
		"echo Extracting...\r\n"+
		"powershell -NoProfile -Command \"Expand-Archive -LiteralPath '%%ZIP%%' -DestinationPath '%%DEST%%' -Force\"\r\n"+
		"del \"%%ZIP%%\" >nul 2>&1\r\n"+
		"set \"EXE=%%DEST%%\\connect-agent.exe\"\r\n"+
		"if not exist \"%%EXE%%\" (\r\n"+
		"  echo connect-agent.exe missing after extract.\r\n"+
		"  pause\r\n"+
		"  exit /b 1\r\n"+
		")\r\n"+
		"echo Enrolling and installing Windows Service (UAC prompt may appear)...\r\n"+
		"powershell -NoProfile -Command \"Start-Process -Wait -Verb RunAs -FilePath '%%EXE%%' -ArgumentList '-server','%%SERVER%%','-enroll','%%CODE%%','-install-service'\"\r\n"+
		"if errorlevel 1 (\r\n"+
		"  echo Service install skipped — starting agent with Startup fallback...\r\n"+
		"  start \"\" \"%%EXE%%\" -server \"%%SERVER%%\" -enroll \"%%CODE%%\"\r\n"+
		") else (\r\n"+
		"  echo OK: ConnectAgent Windows Service is installed and running.\r\n"+
		")\r\n"+
		"timeout /t 4 >nul\r\n",
		wss, code, zipURL)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="Connect-Install.cmd"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, body)
}


func htmlEscape(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&#39;",
	)
	return r.Replace(s)
}

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	base := s.publicBase(r)
	wss := strings.Replace(base, "https://", "wss://", 1)
	wss = strings.Replace(wss, "http://", "ws://", 1)
	if !strings.HasSuffix(wss, "/ws") {
		wss = strings.TrimRight(wss, "/") + "/ws"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `%s`, installPS1(base, wss, code, s.agentPackageAvailable()))
}

func installPS1(base, wss, code string, packageOK bool) string {
	codeLit := powershellSingleQuote(code)
	baseLit := powershellSingleQuote(base)
	wssLit := powershellSingleQuote(wss)
	avail := "$true"
	if !packageOK {
		avail = "$false"
	}
	return fmt.Sprintf(`# Connect agent bootstrap — generated by connectd
$ErrorActionPreference = 'Stop'
$Base = %s
$Server = %s
$Code = %s
$PackageReady = %s

Write-Host "Connect installer"
Write-Host "  Server: $Server"

if (-not $Code) {
  $Code = Read-Host "Enrollment code (ENR-...)"
}
if (-not $Code) { throw "Enrollment code required" }

if (-not $PackageReady) {
  throw "Agent package not published on server. Ask admin to run deploy/publish-agent.ps1"
}

$Dest = Join-Path $env:LOCALAPPDATA 'Connect'
New-Item -ItemType Directory -Force -Path $Dest | Out-Null
$Zip = Join-Path $env:TEMP ('connect-agent-' + [guid]::NewGuid().ToString() + '.zip')
$Url = "$Base/download/agent.zip"
Write-Host "Downloading $Url ..."
Invoke-WebRequest -Uri $Url -OutFile $Zip -UseBasicParsing

# Stop existing agent so files can be replaced
Get-Process -Name connect-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

Expand-Archive -Path $Zip -DestinationPath $Dest -Force
Remove-Item $Zip -Force -ErrorAction SilentlyContinue

$Exe = Join-Path $Dest 'connect-agent.exe'
if (-not (Test-Path $Exe)) {
  # zip may nest a folder
  $found = Get-ChildItem -Path $Dest -Filter connect-agent.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($found) { $Exe = $found.FullName }
}
if (-not (Test-Path $Exe)) { throw "connect-agent.exe missing from package" }

Write-Host "Enrolling and installing Windows Service (UAC may prompt)..."
try {
  $p = Start-Process -FilePath $Exe -ArgumentList @('-server', $Server, '-enroll', $Code, '-install-service') -Verb RunAs -Wait -PassThru
  if ($p.ExitCode -ne 0) { throw "exit $($p.ExitCode)" }
  Write-Host "OK: ConnectAgent Windows Service is installed and running."
} catch {
  Write-Host "Service install skipped — starting agent with Startup fallback..."
  Start-Process -FilePath $Exe -ArgumentList @('-server', $Server, '-enroll', $Code) -WorkingDirectory (Split-Path $Exe)
}
`, baseLit, wssLit, codeLit, avail)
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
