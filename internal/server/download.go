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
	// Prefer new brand name; fall back to older BlueConnect-Setup.exe on VPS.
	primary := filepath.Join(s.agentDir(), "WorthyJoin-Setup.exe")
	if st, err := os.Stat(primary); err == nil && st.Size() > 0 {
		return primary
	}
	legacy := filepath.Join(s.agentDir(), "BlueConnect-Setup.exe")
	if st, err := os.Stat(legacy); err == nil && st.Size() > 0 {
		return legacy
	}
	return primary
}

func (s *Server) installZipPath() string {
	return filepath.Join(s.agentDir(), "WorthyJoin-Install.zip")
}

func (s *Server) agentPackageAvailable() bool {
	st, err := os.Stat(s.agentZipPath())
	return err == nil && st.Size() > 0
}

func (s *Server) setupExeAvailable() bool {
	st, err := os.Stat(s.setupExePath())
	return err == nil && st.Size() > 0
}

func (s *Server) installZipAvailable() bool {
	st, err := os.Stat(s.installZipPath())
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
		"installZip": s.installZipAvailable(),
		"installZipUrl": "/download/install.zip",
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
	if code != "" {
		codeBlock = fmt.Sprintf(
			`<div class="code-panel">
<p class="code-label">Your enrollment code</p>
<code class="big" id="enroll-code">%s</code>
<div class="row"><button type="button" class="btn secondary" id="copy-code">Copy code</button></div>
</div>`,
			htmlEscape(code),
		)
		if available && setupExe {
			primaryBtn = fmt.Sprintf(
				`<a class="btn" href="%s">Download WorthyJoin</a>`,
				htmlEscape(base+"/download/setup.exe"),
			)
		} else if available {
			primaryBtn = fmt.Sprintf(
				`<a class="btn" href="%s">Download WorthyJoin</a>`,
				htmlEscape(base+"/download/setup.cmd?code="+code),
			)
		} else {
			primaryBtn = `<span class="muted-btn">Download WorthyJoin (package missing)</span>`
		}
	} else {
		codeBlock = `<p class="lede">Open this page from the install link your tech sent.</p>`
		if available && setupExe {
			primaryBtn = fmt.Sprintf(
				`<a class="btn" href="%s">Download WorthyJoin</a>`,
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
<title>Install WorthyJoin</title>
<style>
:root{
  --bg:#0a0a0a;--pane:#121212;--line:#2a2a2a;--text:#f2f2f2;--muted:#8a8a8a;
  --accent:#3b82f6;--accent2:#2563eb;--ok:#22c55e;--warn:#fbbf24;
  --font:"Segoe UI","Helvetica Neue",sans-serif;
}
*{box-sizing:border-box}
html,body{margin:0;min-height:100%%;font:15px/1.45 var(--font);color:var(--text);background:var(--bg)}
body{
  background:
    radial-gradient(ellipse 90%% 55%% at 50%% -15%%, rgba(59,130,246,.22), transparent 55%%),
    radial-gradient(ellipse 60%% 40%% at 100%% 100%%, rgba(0,0,0,.35), transparent 50%%),
    var(--bg);
}
.page{max-width:720px;margin:0 auto;padding:36px 20px 56px;display:grid;gap:18px}
.hero{
  background:linear-gradient(180deg,#161616 0%%,var(--pane) 100%%);
  border:1px solid var(--line);border-radius:16px;padding:28px 28px 24px;
  display:grid;gap:16px;
  box-shadow:0 18px 50px rgba(0,0,0,.35);
}
.brand{display:flex;align-items:center;gap:12px}
.mark{
  width:36px;height:36px;border-radius:9px;background:var(--accent);
  display:grid;place-items:center;color:#fff;font-weight:800;font-size:16px;
  box-shadow:0 0 0 4px rgba(59,130,246,.15);
}
.brand h1{margin:0;font-size:26px;letter-spacing:-.04em;font-weight:700}
.lede{margin:0;color:var(--muted);font-size:14px;max-width:38em}
.ok{color:var(--ok);margin:0;font-size:13px;font-weight:600}
.warn{
  color:var(--warn);margin:0;font-size:13px;line-height:1.4;
  background:rgba(251,191,36,.08);border:1px solid rgba(251,191,36,.22);
  padding:10px 12px;border-radius:10px;
}
.code-panel{display:grid;gap:8px}
.code-label{margin:0;font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:var(--muted)}
.big{
  font:700 14px Consolas,ui-monospace,monospace;word-break:break-all;
  background:#0a0a0a;border:1px solid var(--line);padding:12px 14px;border-radius:10px;display:block;
}
.row{display:flex;flex-wrap:wrap;gap:10px;align-items:center}
.btn{
  display:inline-flex;align-items:center;justify-content:center;gap:8px;
  text-decoration:none;border:0;background:var(--accent);color:#fff;
  font:inherit;font-weight:700;padding:13px 18px;border-radius:10px;cursor:pointer;
  min-height:46px;
}
.btn:hover{background:var(--accent2)}
.btn.secondary{background:#1a1a1a;color:var(--text);border:1px solid #333}
.btn.secondary:hover{background:#222}
.muted-btn{
  background:#333;pointer-events:none;display:inline-flex;align-items:center;
  padding:13px 18px;border-radius:10px;color:#bbb;font-weight:700;min-height:46px;
}
.steps{
  margin:0;padding:0;list-style:none;display:grid;gap:8px;
  counter-reset:step;
}
.steps li{
  counter-increment:step;display:grid;grid-template-columns:28px 1fr;gap:10px;align-items:start;
  color:#b3b3b3;font-size:13.5px;line-height:1.4;
}
.steps li::before{
  content:counter(step);width:28px;height:28px;border-radius:8px;
  background:#1a1a1a;border:1px solid var(--line);color:#d4d4d4;
  display:grid;place-items:center;font-size:12px;font-weight:700;
}
.steps strong{color:#eee;font-weight:650}
.foot{margin:0;color:#666;font-size:12px}

.help{
  border:1px solid rgba(251,191,36,.35);
  border-radius:16px;
  background:linear-gradient(180deg, rgba(251,191,36,.08), rgba(18,18,18,.96));
  padding:20px 20px 18px;
  display:grid;gap:14px;
}
.help-head{display:grid;gap:6px}
.help-kicker{
  margin:0;font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:var(--warn);
}
.help-title{margin:0;font-size:18px;letter-spacing:-.02em;font-weight:700}
.help-intro{margin:0;color:#c4c4c4;font-size:13.5px;line-height:1.45}
.grid{display:grid;grid-template-columns:1fr;gap:12px}
@media (min-width:700px){
  .grid{grid-template-columns:1fr 1fr;gap:12px}
  .grid .tile.wide{grid-column:1 / -1}
}
@media (max-width:720px){
  .hero{padding:22px 18px}
  .brand h1{font-size:22px}
}
.tile{
  background:#0e0e0e;border:1px solid var(--line);border-radius:12px;overflow:hidden;
  display:grid;gap:0;
}
.tile .meta{padding:12px 14px 14px;display:grid;gap:6px}
.tile .meta b{font-size:14px;letter-spacing:-.01em}
.tile .meta span{font-size:13px;color:#bdbdbd;line-height:1.4}
.tile .do{
  display:inline-flex;align-items:center;gap:6px;margin-top:2px;
  font-size:13px;font-weight:700;color:#fff;
}
.tile .do em{font-style:normal;color:var(--warn)}

/* Lightweight warning mockups — crisp, instant, no large images */
.mock{padding:14px;background:#1b1b1b;border-bottom:1px solid var(--line)}
.chrome-bar,.edge-bar{
  display:flex;align-items:center;gap:10px;flex-wrap:wrap;
  background:#fff;color:#202124;border-radius:8px;padding:10px 12px;
  font:13px/1.3 "Segoe UI",system-ui,sans-serif;
  box-shadow:0 4px 16px rgba(0,0,0,.25);
}
.warn-ico{
  flex:0 0 auto;width:22px;height:22px;border-radius:50%%;
  background:#f9ab00;color:#202124;display:grid;place-items:center;
  font-weight:800;font-size:13px;
}
.chrome-bar .msg,.edge-bar .msg{flex:1 1 180px;min-width:0}
.chrome-bar .msg strong,.edge-bar .msg strong{display:block;font-size:12px;font-weight:700}
.chrome-bar .msg span,.edge-bar .msg span{font-size:12px;color:#5f6368}
.chrome-bar .acts,.edge-bar .acts{display:flex;gap:8px;margin-left:auto}
.pill{
  border:0;border-radius:16px;padding:6px 12px;font:650 12px "Segoe UI",system-ui,sans-serif;cursor:default;
}
.pill.ghost{background:#fff;color:#1a73e8;border:1px solid #dadce0}
.pill.solid{background:#1a73e8;color:#fff}
.edge-bar .pill.ghost{color:#0067c0;border-color:#d0d0d0}
.edge-bar .pill.solid{background:#0067c0}

.ss-dlg{
  max-width:360px;margin:0 auto;background:#fff;color:#1b1b1b;border-radius:8px;
  padding:16px 16px 14px;font:13px/1.35 "Segoe UI",system-ui,sans-serif;
  box-shadow:0 8px 28px rgba(0,0,0,.35);
}
.ss-dlg .shield{
  width:36px;height:36px;border-radius:8px;background:#0078d4;color:#fff;
  display:grid;place-items:center;font-size:18px;font-weight:800;margin-bottom:10px;
}
.ss-dlg h3{margin:0 0 6px;font-size:15px;font-weight:700}
.ss-dlg p{margin:0 0 8px;color:#5a5a5a;font-size:12.5px}
.ss-dlg .app{margin:0 0 10px;font-size:12px;color:#323232}
.ss-dlg .app b{font-weight:650}
.ss-dlg .links{display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap}
.ss-dlg .link{color:#0067c0;font-weight:650;font-size:12.5px}
.ss-dlg .run{
  background:#0067c0;color:#fff;border:0;border-radius:4px;padding:6px 12px;
  font:650 12px "Segoe UI",system-ui,sans-serif;
}
.kbd{
  font:12px Consolas,ui-monospace,monospace;background:#1a1a1a;border:1px solid #333;
  border-radius:4px;padding:1px 6px;color:#e5e5e5;
}
</style>
</head>
<body>
<main class="page">
  <section class="hero">
    <div class="brand"><span class="mark" aria-hidden="true">★</span><h1>WorthyJoin</h1></div>
    <p class="lede">Install on this Windows PC. Download the installer, open it, and paste your enrollment code when asked.</p>
    %s
    %s
    <div class="row">%s</div>
    <ol class="steps">
      <li><span>Click <strong>Download WorthyJoin</strong>.</span></li>
      <li><span>Open the file. If Windows warns, choose <strong>More info</strong> → <strong>Run anyway</strong>.</span></li>
      <li><span>Paste the enrollment code if asked, then <strong>Install</strong>. Approve any permission prompt with <strong>Yes</strong>.</span></li>
    </ol>
    <p class="foot">Windows only</p>
  </section>

  <section class="help" aria-label="Download warning guide">
    <div class="help-head">
      <p class="help-kicker">Important · expected warning</p>
      <h2 class="help-title">If Chrome, Edge, or Windows warns you</h2>
      <p class="help-intro">New installers often look “uncommon” until we have a publisher certificate. That is normal for WorthyJoin right now — keep the file and run it.</p>
    </div>
    <div class="grid">
      <article class="tile">
        <div class="mock" aria-hidden="true">
          <div class="chrome-bar">
            <span class="warn-ico">!</span>
            <div class="msg">
              <strong>WorthyJoin-Setup.exe</strong>
              <span>This file isn’t commonly downloaded and may be dangerous.</span>
            </div>
            <div class="acts">
              <span class="pill ghost">Keep</span>
              <span class="pill solid">Discard</span>
            </div>
          </div>
        </div>
        <div class="meta">
          <b>Google Chrome</b>
          <span>In the download bar, click <strong>Keep</strong>. If it asks again, choose <strong>Keep anyway</strong>.</span>
          <span class="do">Do this → <em>Keep</em></span>
        </div>
      </article>
      <article class="tile">
        <div class="mock" aria-hidden="true">
          <div class="edge-bar">
            <span class="warn-ico">!</span>
            <div class="msg">
              <strong>WorthyJoin-Setup.exe</strong>
              <span>This file isn’t commonly downloaded and may be dangerous.</span>
            </div>
            <div class="acts">
              <span class="pill ghost">Keep</span>
              <span class="pill solid">Delete</span>
            </div>
          </div>
        </div>
        <div class="meta">
          <b>Microsoft Edge</b>
          <span>Choose <strong>Keep</strong>. If needed: <strong>Show more</strong> → <strong>Keep anyway</strong>.</span>
          <span class="do">Do this → <em>Keep</em></span>
        </div>
      </article>
      <article class="tile wide">
        <div class="mock" aria-hidden="true">
          <div class="ss-dlg">
            <div class="shield">✓</div>
            <h3>Windows protected your PC</h3>
            <p>Microsoft Defender SmartScreen prevented an unrecognized app from starting.</p>
            <p class="app"><b>App:</b> WorthyJoin-Setup.exe &nbsp;·&nbsp; <b>Publisher:</b> Unknown</p>
            <div class="links">
              <span class="link">More info</span>
              <span class="run">Run anyway</span>
            </div>
          </div>
        </div>
        <div class="meta">
          <b>Windows SmartScreen</b>
          <span>When you open the installer: click <span class="kbd">More info</span>, then <span class="kbd">Run anyway</span>.</span>
          <span class="do">Do this → <em>More info → Run anyway</em></span>
        </div>
      </article>
    </div>
  </section>
</main>
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
</body></html>`, status, codeBlock, primaryBtn)
}

func (s *Server) handleDownloadInstallZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.installZipPath()
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "WorthyJoin-Install.zip not published yet — run deploy/publish-agent.ps1", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="WorthyJoin-Install.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func (s *Server) handleDownloadSetupExe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.setupExePath()
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "WorthyJoin-Setup.exe not published yet — run deploy/publish-agent.ps1", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="WorthyJoin-Setup.exe"`)
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
		"title WorthyJoin Install\r\n"+
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
