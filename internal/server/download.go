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

func (s *Server) setupExePath() string {
	return filepath.Join(s.agentDir(), "BlueConnect-Setup.exe")
}

func (s *Server) agentPackageAvailable() bool {
	st, err := os.Stat(s.agentZipPath())
	return err == nil && st.Size() > 0
}

func (s *Server) setupExeAvailable() bool {
	st, err := os.Stat(s.setupExePath())
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
		"available":  true,
		"size":       st.Size(),
		"updatedAt":  st.ModTime().UTC().Format(time.RFC3339),
		"download":   "/download/agent.zip",
		"setupExe":   s.setupExeAvailable(),
		"setupUrl":   "/download/setup.exe",
		"install":    "/install",
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
	setupExe := s.setupExeAvailable()

	status := `<p class="ok">Ready to install on this Windows PC.</p>`
	if !available {
		status = `<p class="warn">The install package is not on the server yet. Ask your admin to publish it (Admin → Agent package).</p>`
	}

	codeBlock := ""
	primaryBtn := ""
	legacy := ""
	if code != "" {
		codeBlock = fmt.Sprintf(
			`<p class="code-label">Your enrollment code</p>
<code class="big" id="enroll-code">%s</code>
<button type="button" class="btn secondary" id="copy-code">Copy code</button>`,
			htmlEscape(code),
		)
		if available && setupExe {
			primaryBtn = fmt.Sprintf(
				`<a class="btn" href="%s">Download BlueConnect</a>`,
				htmlEscape(base+"/download/setup.exe"),
			)
		} else if available {
			primaryBtn = fmt.Sprintf(
				`<a class="btn" href="%s">Download BlueConnect</a>`,
				htmlEscape(base+"/download/setup.cmd?code="+code),
			)
		} else {
			primaryBtn = `<span class="btn muted-btn">Download BlueConnect (package missing)</span>`
		}
		if available {
			legacy = fmt.Sprintf(
				`<p class="legacy"><a href="%s">Having trouble? Use the legacy installer</a></p>`,
				htmlEscape(base+"/download/setup.cmd?code="+code),
			)
		}
	} else {
		codeBlock = `<p class="muted">Open this page from the install link your tech sent.</p>`
		if available && setupExe {
			primaryBtn = fmt.Sprintf(
				`<a class="btn" href="%s">Download BlueConnect</a>`,
				htmlEscape(base+"/download/setup.exe"),
			)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Install BlueConnect</title>
<style>
body{font:15px/1.45 "Segoe UI",system-ui,sans-serif;margin:0;background:#0a0a0a;color:#f2f2f2}
.wrap{max-width:440px;margin:48px auto;padding:0 16px}
.card{background:#141414;border:1px solid #2a2a2a;border-radius:14px;padding:26px;display:grid;gap:14px}
.brand{display:flex;align-items:center;gap:10px}
.mark{width:28px;height:28px;border-radius:7px;background:#3b82f6;display:grid;place-items:center;color:#fff;font-weight:700}
h1{margin:0;font-size:22px;letter-spacing:-.03em}
.credit{font:italic 11px Georgia,serif;letter-spacing:.08em;color:#b8956a}
.muted{color:#8a8a8a;margin:0}
.ok{color:#22c55e;margin:0}
.warn{color:#fbbf24;margin:0;background:rgba(251,191,36,.08);border:1px solid rgba(251,191,36,.25);padding:10px;border-radius:8px}
.code-label{margin:0;font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.05em;color:#8a8a8a}
.big{font:700 15px Consolas,monospace;word-break:break-all;background:#0a0a0a;border:1px solid #2a2a2a;padding:12px;border-radius:8px;display:block}
.steps{margin:0;padding-left:18px;color:#a3a3a3}
.steps li{margin:8px 0}
.btn{display:inline-block;text-align:center;text-decoration:none;border:0;background:#3b82f6;color:#fff;font:inherit;font-weight:700;padding:13px 14px;border-radius:8px;cursor:pointer}
.btn:hover{background:#2563eb}
.btn.secondary{background:#1a1a1a;color:#f2f2f2;border:1px solid #333}
.btn.secondary:hover{background:#222}
.muted-btn{background:#404040;pointer-events:none;display:inline-block;padding:13px 14px;border-radius:8px;color:#fff;font-weight:700}
.actions{display:grid;gap:10px}
.legacy{margin:0;font-size:12px;text-align:center}
.legacy a{color:#8a8a8a}
</style>
</head>
<body>
<div class="wrap"><div class="card">
<div class="brand"><span class="mark">★</span><div><h1>BlueConnect</h1><div class="credit">by godFather</div></div></div>
<p class="muted">Install on this Windows PC in a few clicks.</p>
%s
%s
<div class="actions">%s</div>
%s
<ol class="steps">
<li>Click <strong>Download BlueConnect</strong>.</li>
<li>Open the file you downloaded.</li>
<li>Paste your enrollment code if asked, then click <strong>Install</strong>.</li>
<li>Allow Windows prompts if they appear — this PC shows online in the Host console.</li>
</ol>
<p class="muted">Windows only.</p>
</div></div>
<script>
(function(){
  var btn=document.getElementById('copy-code');
  var code=document.getElementById('enroll-code');
  if(!btn||!code)return;
  btn.onclick=async function(){
    try{await navigator.clipboard.writeText(code.textContent.trim());btn.textContent='Copied';}
    catch(e){btn.textContent='Select & copy manually';}
  };
})();
</script>
</body></html>`, status, codeBlock, primaryBtn, legacy)
}

func (s *Server) handleDownloadSetupExe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.setupExePath()
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "BlueConnect-Setup.exe not published yet — run deploy/publish-agent.ps1", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="BlueConnect-Setup.exe"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
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
	zipURL := base + "/download/agent.zip"

	// Success = config.json has tenantId. GUI subsystem exit codes are unreliable
	// ($null -ne 0 is true in PowerShell and caused false "Enrollment failed").
	psEnroll := "$ErrorActionPreference='Stop';" +
		"$exe=$env:EXE;$dir=Split-Path -Parent $exe;$cfg=Join-Path $dir 'config.json';" +
		"if(Test-Path -LiteralPath $cfg){try{$j=Get-Content -LiteralPath $cfg -Raw|ConvertFrom-Json;if($j.tenantId){Write-Host Already enrolled;exit 0}}catch{}};" +
		"$p=Start-Process -FilePath $exe -WorkingDirectory $dir -Wait -PassThru -ArgumentList @('-server',$env:SERVER,'-enroll',$env:CODE,'-quit-after-enroll');" +
		"if(Test-Path -LiteralPath $cfg){try{$j=Get-Content -LiteralPath $cfg -Raw|ConvertFrom-Json;if($j.tenantId){exit 0}}catch{}};" +
		"if($null -ne $p -and $null -ne $p.ExitCode -and [int]$p.ExitCode -ne 0){exit [int]$p.ExitCode};" +
		"exit 1"
	psSvc := "try{" +
		"$p=Start-Process -FilePath $env:EXE -Verb RunAs -Wait -PassThru -ArgumentList @('-install-service');" +
		"if($null -ne $p -and $null -ne $p.ExitCode -and [int]$p.ExitCode -ne 0){exit [int]$p.ExitCode};" +
		"exit 0" +
		"}catch{exit 1}"

	body := fmt.Sprintf("@echo off\r\n"+
		"setlocal\r\n"+
		"title BlueConnect Install\r\n"+
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
		"echo Stopping existing agent/service...\r\n"+
		"sc stop ConnectAgent >nul 2>&1\r\n"+
		"taskkill /IM connect-agent.exe /F >nul 2>&1\r\n"+
		"timeout /t 2 /nobreak >nul\r\n"+
		"echo Extracting...\r\n"+
		"powershell -NoProfile -Command \"Expand-Archive -LiteralPath '%%ZIP%%' -DestinationPath '%%DEST%%' -Force\"\r\n"+
		"del \"%%ZIP%%\" >nul 2>&1\r\n"+
		"set \"EXE=%%DEST%%\\connect-agent.exe\"\r\n"+
		"if not exist \"%%EXE%%\" (\r\n"+
		"  echo connect-agent.exe missing after extract.\r\n"+
		"  pause\r\n"+
		"  exit /b 1\r\n"+
		")\r\n"+
		"echo Enrolling this PC (no admin needed)...\r\n"+
		"powershell -NoProfile -ExecutionPolicy Bypass -Command \"%s\"\r\n"+
		"if errorlevel 1 (\r\n"+
		"  echo Enrollment failed. Check %%DEST%%\\enroll.log or issue a fresh enrollment code.\r\n"+
		"  if exist \"%%DEST%%\\enroll.log\" type \"%%DEST%%\\enroll.log\"\r\n"+
		"  pause\r\n"+
		"  exit /b 1\r\n"+
		")\r\n"+
		"echo Installing Windows Service (UAC prompt may appear)...\r\n"+
		"powershell -NoProfile -ExecutionPolicy Bypass -Command \"%s\"\r\n"+
		"if errorlevel 1 (\r\n"+
		"  echo Service install skipped — starting agent with Startup fallback...\r\n"+
		"  start \"\" \"%%EXE%%\"\r\n"+
		") else (\r\n"+
		"  echo OK: ConnectAgent Windows Service is installed and running.\r\n"+
		")\r\n"+
		"timeout /t 4 >nul\r\n",
		wss, code, zipURL, psEnroll, psSvc)

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

Write-Host "Stopping existing agent/service..."
try { Stop-Service -Name ConnectAgent -Force -ErrorAction SilentlyContinue } catch {}
Get-Process -Name connect-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

Expand-Archive -Path $Zip -DestinationPath $Dest -Force
Remove-Item $Zip -Force -ErrorAction SilentlyContinue

$Exe = Join-Path $Dest 'connect-agent.exe'
if (-not (Test-Path $Exe)) {
  # zip may nest a folder
  $found = Get-ChildItem -Path $Dest -Filter connect-agent.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($found) { $Exe = $found.FullName }
}
if (-not (Test-Path $Exe)) { throw "connect-agent.exe missing from package" }

Write-Host "Enrolling this PC (no admin needed)..."
$dir = Split-Path -Parent $Exe
$cfg = Join-Path $dir 'config.json'
function Test-Enrolled {
  if (-not (Test-Path -LiteralPath $cfg)) { return $false }
  try {
    $j = Get-Content -LiteralPath $cfg -Raw | ConvertFrom-Json
    return [bool]$j.tenantId
  } catch { return $false }
}
if (Test-Enrolled) {
  Write-Host "Already enrolled — skipping redeem."
} else {
  $enroll = Start-Process -FilePath $Exe -WorkingDirectory $dir -ArgumentList @('-server', $Server, '-enroll', $Code, '-quit-after-enroll') -Wait -PassThru
  if (-not (Test-Enrolled)) {
    $code = 1
    if ($null -ne $enroll -and $null -ne $enroll.ExitCode) { $code = [int]$enroll.ExitCode }
    $log = Join-Path $dir 'enroll.log'
    if (Test-Path -LiteralPath $log) { Get-Content -LiteralPath $log | Write-Host }
    throw "Enrollment failed (exit $code). Issue a fresh enrollment code if this one was already used."
  }
}

Write-Host "Installing Windows Service (UAC may prompt)..."
try {
  $p = Start-Process -FilePath $Exe -ArgumentList @('-install-service') -Verb RunAs -Wait -PassThru
  if ($null -ne $p -and $null -ne $p.ExitCode -and [int]$p.ExitCode -ne 0) { throw "exit $($p.ExitCode)" }
  Write-Host "OK: ConnectAgent Windows Service is installed and running."
} catch {
  Write-Host "Service install skipped — starting agent with Startup fallback..."
  Start-Process -FilePath $Exe -WorkingDirectory $dir
}
`, baseLit, wssLit, codeLit, avail)
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
