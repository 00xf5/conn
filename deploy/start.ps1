# Connect — build server + agent, then start both. Only script you need.
param(
  [switch]$Console,
  [switch]$Restart,
  [switch]$Firewall,
  [switch]$Build
)

$ErrorActionPreference = "Stop"

function Get-RepoRoot {
  if (Test-Path (Join-Path $PSScriptRoot "..\connect-agent.exe")) {
    return (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
  }
  if (Test-Path (Join-Path $env:LOCALAPPDATA "Connect\connect-agent.exe")) {
    return Join-Path $env:LOCALAPPDATA "Connect"
  }
  return (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
}

function Get-LanIP {
  $ip = (
    Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -notmatch '^127\.' -and $_.PrefixOrigin -ne 'WellKnown' } |
    Select-Object -First 1 -ExpandProperty IPAddress
  )
  if ($ip) { return $ip }
  return "127.0.0.1"
}

function Get-Config {
  foreach ($p in @(
    (Join-Path $env:LOCALAPPDATA "Connect\config.json"),
    (Join-Path (Get-RepoRoot) "config.json")
  )) {
    if (Test-Path $p) { return Get-Content $p -Raw | ConvertFrom-Json }
  }
  return $null
}

function Ensure-Firewall {
  param([int]$HttpPort = 8787, [int]$TurnPort = 3478)
  foreach ($r in @(
    @{ Name = "Connect Server (TCP $HttpPort)"; Protocol = "TCP"; Port = $HttpPort },
    @{ Name = "Connect TURN (UDP $TurnPort)"; Protocol = "UDP"; Port = $TurnPort }
  )) {
    if (Get-NetFirewallRule -DisplayName $r.Name -ErrorAction SilentlyContinue) {
      Write-Host "  Firewall OK: $($r.Name)"
      continue
    }
    New-NetFirewallRule -DisplayName $r.Name -Direction Inbound -Action Allow `
      -Protocol $r.Protocol -LocalPort $r.Port -Profile Any | Out-Null
    Write-Host "  Firewall added: $($r.Name)"
  }
}

function Stop-ConnectProcesses {
  foreach ($name in @('connect-agent', 'connectd')) {
    Stop-Process -Name $name -Force -ErrorAction SilentlyContinue
  }
  Start-Sleep -Seconds 2
  foreach ($name in @('connect-agent', 'connectd')) {
    $procs = @(Get-Process -Name $name -ErrorAction SilentlyContinue)
    if ($procs.Count -eq 0) { continue }
    foreach ($p in $procs) {
      try { Stop-Process -Id $p.Id -Force -ErrorAction Stop } catch {}
    }
  }
  Start-Sleep -Seconds 1
}

function Assert-ConnectStopped {
  $agents = @(Get-Process -Name connect-agent -ErrorAction SilentlyContinue)
  $servers = @(Get-Process -Name connectd -ErrorAction SilentlyContinue)
  if ($agents.Count -eq 0 -and $servers.Count -eq 0) { return }

  Write-Host ""
  Write-Host "ERROR: could not stop Connect processes (need Administrator)."
  if ($agents.Count -gt 0) {
    Write-Host "  connect-agent still running: $($agents.Id -join ', ')"
  }
  if ($servers.Count -gt 0) {
    Write-Host "  connectd still running: $($servers.Id -join ', ')"
  }
  Write-Host ""
  Write-Host "Fix: open PowerShell as Administrator, then run:"
  Write-Host "  cd $Root"
  Write-Host "  .\deploy\start.ps1 -Restart -Firewall"
  Write-Host ""
  exit 1
}

function Assert-SingleAgent {
  $agents = @(Get-Process -Name connect-agent -ErrorAction SilentlyContinue)
  if ($agents.Count -le 1) { return }
  Write-Host ""
  Write-Host "ERROR: $($agents.Count) connect-agent processes running (PIDs: $($agents.Id -join ', '))."
  Write-Host "They fight over the same device ID and the dashboard shows 0 agents."
  Write-Host "Run as Administrator: .\deploy\start.ps1 -Restart -Firewall"
  Write-Host ""
  exit 1
}

$Root = Get-RepoRoot
Set-Location $Root

if ($Build) {
  Write-Host "Building connectd..."
  go build -o connectd.exe ./cmd/connectd
  $gcc = (Get-Command gcc -ErrorAction SilentlyContinue).Source
  if (-not $gcc) {
    $wingetGcc = Get-ChildItem "$env:LOCALAPPDATA\Microsoft\WinGet\Packages" -Filter gcc.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($wingetGcc) { $env:PATH = "$(Split-Path $wingetGcc.FullName -Parent);$env:PATH" }
  }
  $env:CGO_ENABLED = "1"
  Write-Host "Building connect-agent..."
  go run ./cmd/connect-agent/genicon.go
  go build -ldflags "-H=windowsgui" -o connect-agent.exe ./cmd/connect-agent
  Write-Host "Built connectd.exe + connect-agent.exe"
}

$ff = Get-ChildItem "$env:LOCALAPPDATA\Microsoft\WinGet\Packages" -Filter ffmpeg.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
if ($ff) {
  $env:CONNECT_ALLOW_SYSTEM_FFMPEG = "1"
  $env:PATH = "$(Split-Path $ff.FullName -Parent);$env:PATH"
}

if ($Restart) {
  Write-Host "Stopping connectd + connect-agent..."
  Stop-ConnectProcesses
  Assert-ConnectStopped
}

if ($Firewall) {
  try { Ensure-Firewall } catch { Write-Host "  Firewall needs Administrator - phones may not reach this PC" }
}

# --- connectd ---
$connectd = Join-Path $Root "connectd.exe"
if (Test-Path $connectd) {
  $serverUp = $false
  if (-not $Restart) {
    try {
      $null = Invoke-RestMethod -Uri "http://127.0.0.1:8787/api/health" -TimeoutSec 2
      $serverUp = $true
    } catch {}
  }
  if (-not $serverUp) {
    $ip = Get-LanIP
    Write-Host "Starting connectd -> http://${ip}:8787"
    Start-Process -FilePath $connectd -ArgumentList @("-no-tls", "-public-url", "http://${ip}:8787") -WindowStyle Hidden
    Start-Sleep -Seconds 2
  } else {
    Write-Host "connectd already running"
  }
}

# --- connect-agent ---
Assert-SingleAgent
if (Get-Process connect-agent -ErrorAction SilentlyContinue) {
  Write-Host "connect-agent already running - tray icon"
} else {
  $agentExe = Join-Path $Root "connect-agent.exe"
  if (-not (Test-Path $agentExe)) { throw "connect-agent.exe not found - run: .\deploy\start.ps1 -Build" }

  $cfg = Get-Config
  $server = if ($cfg.serverUrl) { [string]$cfg.serverUrl } else { "ws://$(Get-LanIP):8787/ws" }
  $insecure = if ($null -ne $cfg.insecureTls) { [bool]$cfg.insecureTls } else { $true }
  $agentArgs = @("-server", $server)
  if ($server -match '^wss://' -and $insecure) { $agentArgs += "-insecure-tls" }

  if ($Console) {
    Write-Host "Starting connect-agent (console) -> $server"
    & $agentExe @agentArgs
    exit $LASTEXITCODE
  }
  Write-Host "Starting connect-agent - tray -> $server"
  Start-Process -FilePath $agentExe -ArgumentList $agentArgs -WindowStyle Hidden
}

$ip = Get-LanIP
Write-Host ""
try {
  $health = Invoke-RestMethod -Uri "http://127.0.0.1:8787/api/health" -TimeoutSec 3
  Write-Host "Connect running. Agents online: $($health.agents)"
} catch {
  Write-Host "Connect started (server health check pending)."
}
Write-Host "  Dashboard: http://${ip}:8787/dashboard/"
Write-Host "  Log:       $env:LOCALAPPDATA\Connect\agent.log"
