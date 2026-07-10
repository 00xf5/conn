package server

import (
	"fmt"
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
			"hint":      "Upload agent.zip via deploy/publish-agent.ps1 (mount CONNECT_AGENT_DIR)",
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
	psCmd := fmt.Sprintf(`irm %s/install.ps1`, base)
	if code != "" {
		psCmd = fmt.Sprintf(`irm '%s/install.ps1?code=%s' | iex`, base, code)
	} else {
		psCmd = fmt.Sprintf(`irm %s/install.ps1 | iex`, base)
	}

	status := `<p class="ok">Agent package ready.</p>`
	if !available {
		status = `<p class="warn">Agent package is not on the server yet. Ask your admin to publish it (<code>deploy/publish-agent.ps1</code>).</p>`
	}

	codeBlock := ""
	if code != "" {
		codeBlock = fmt.Sprintf(`<p class="code-label">Enrollment code</p><code class="big">%s</code>`, htmlEscape(code))
	} else {
		codeBlock = `<p class="muted">Open this page from an enrollment link, or paste a code into the PowerShell command your tech sent.</p>`
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
.cmd{font:13px Consolas,monospace;background:#1e2430;color:#e8eaee;padding:12px;border-radius:4px;word-break:break-all;display:block}
button{border:0;background:#0b5fff;color:#fff;font:inherit;font-weight:700;padding:10px 14px;border-radius:4px;cursor:pointer}
button:hover{background:#094fd6}
.steps{margin:0;padding-left:18px}
.steps li{margin:6px 0}
</style>
</head>
<body>
<div class="wrap"><div class="card">
<h1>Install Connect</h1>
<p class="muted">Remote access agent for this organization. One-time setup on this PC.</p>
%s
%s
<ol class="steps">
<li>Open <strong>PowerShell</strong> (Start → type PowerShell).</li>
<li>Paste the command below and press Enter.</li>
<li>When it finishes, this PC appears in the Host console.</li>
</ol>
<code class="cmd" id="cmd">%s</code>
<button type="button" id="copy">Copy install command</button>
<p class="muted">Windows only. Requires network access to this server.</p>
</div></div>
<script>
document.getElementById('copy').onclick=async()=>{
  const t=document.getElementById('cmd').textContent;
  try{await navigator.clipboard.writeText(t);document.getElementById('copy').textContent='Copied';}
  catch{prompt('Copy this command',t);}
};
</script>
</body></html>`, status, codeBlock, htmlEscape(psCmd))
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

Write-Host "Enrolling and starting..."
$args = @('-server', $Server, '-enroll', $Code)
Start-Process -FilePath $Exe -ArgumentList $args -WorkingDirectory (Split-Path $Exe)
Write-Host "Done. This PC should appear in the Host console shortly."
`, baseLit, wssLit, codeLit, avail)
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
