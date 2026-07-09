# Point connect-agent at your VPS (connectd + coturn). Run on the Windows host PC.
param(
  [Parameter(Mandatory = $true)]
  [string]$Server
)

$ErrorActionPreference = "Stop"
$Root = Split-Path $PSScriptRoot -Parent
$Agent = Join-Path $Root "connect-agent.exe"
if (-not (Test-Path $Agent)) {
  throw "connect-agent.exe not found - run: .\deploy\start.ps1 -Build"
}

if ($Server -notmatch '^wss://') {
  throw "Server must be wss://YOUR_DOMAIN/ws (HTTPS required for internet viewers)"
}

$configDir = Join-Path $env:LOCALAPPDATA "Connect"
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
$configPath = Join-Path $configDir "config.json"

$configJson = @{
  serverUrl   = $Server
  insecureTls = $false
  width       = 854
  height      = 480
  fps         = 20
  bitrate     = 2000
  gop         = 40
  keyIntMin   = 20
} | ConvertTo-Json
$utf8NoBom = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($configPath, $configJson, $utf8NoBom)

Write-Host "Config written: $configPath"
Write-Host "Server: $Server"
Write-Host "TURN: provisioned by connectd (CONNECT_TURN_URL on VPS)"

Remove-Item Env:CONNECT_ENCODER_FFMPEG -ErrorAction SilentlyContinue
Remove-Item Env:CONNECT_ENCODER_GDIGRAB -ErrorAction SilentlyContinue

Stop-Process -Name connect-agent -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
Start-Process -FilePath $Agent -ArgumentList @("-server", $Server) -WindowStyle Hidden

Write-Host "connect-agent started"
Write-Host "Log: $(Join-Path $configDir 'agent.log')"
Write-Host ""
Write-Host "Viewer: open https://YOUR_DOMAIN/dashboard/ (same domain as wss URL, without /ws)"
Write-Host "Test cellular: disable Wi-Fi on phone, use LTE/5G"
