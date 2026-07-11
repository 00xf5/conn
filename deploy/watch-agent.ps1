# Keep connect-agent alive: restart on crash or exit. Run in a dedicated PowerShell window.
param(
  [string]$Server = "wss://worthyjoin.online/ws"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path $PSScriptRoot -Parent
$Agent = Join-Path $Root "connect-agent.exe"
if (-not (Test-Path $Agent)) {
  throw "connect-agent.exe not found - run: .\deploy\start.ps1 -Build"
}

$configDir = Join-Path $env:LOCALAPPDATA "Connect"
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
$configPath = Join-Path $configDir "config.json"

$configJson = @{
  serverUrl   = $Server
  insecureTls = $false
  width       = 1280
  height      = 720
  fps         = 20
  bitrate     = 4500
  gop         = 40
  keyIntMin   = 20
} | ConvertTo-Json
$utf8NoBom = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($configPath, $configJson, $utf8NoBom)

Remove-Item Env:CONNECT_ENCODER_FFMPEG -ErrorAction SilentlyContinue
Remove-Item Env:CONNECT_ENCODER_GDIGRAB -ErrorAction SilentlyContinue

Write-Host "Watchdog: $Agent -> $Server"
Write-Host "Log: $(Join-Path $configDir 'agent.log')"
Write-Host "Press Ctrl+C to stop the watchdog (agent will keep running until next restart)."

while ($true) {
  $proc = Start-Process -FilePath $Agent -ArgumentList @("-server", $Server, "-console") -PassThru -NoNewWindow
  Write-Host "$(Get-Date -Format o) agent started pid=$($proc.Id)"
  Wait-Process -Id $proc.Id
  Write-Host "$(Get-Date -Format o) agent exited pid=$($proc.Id); restart in 3s"
  Start-Sleep -Seconds 3
}
