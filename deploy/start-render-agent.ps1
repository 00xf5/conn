# Start connect-agent pointed at Render (signaling only on cloud).

param(

  [string]$Server = "wss://blueconnect.online/ws"

)



$ErrorActionPreference = "Stop"

$Root = Split-Path $PSScriptRoot -Parent

$Agent = Join-Path $Root "connect-agent.exe"

if (-not (Test-Path $Agent)) {

  throw "connect-agent.exe not found - run deploy/start.ps1 -Build"

}



$ff = Join-Path $Root "bin\ffmpeg.exe"

if (-not (Test-Path $ff)) {

  $winget = Get-ChildItem "$env:LOCALAPPDATA\Microsoft\WinGet\Packages" -Filter ffmpeg.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1

  if ($winget) {

    New-Item -ItemType Directory -Force -Path (Split-Path $ff) | Out-Null

    Copy-Item $winget.FullName $ff -Force

    Write-Host "Copied ffmpeg to bin\ffmpeg.exe"

  } else {

    Write-Host "WARN: bin\ffmpeg.exe missing - video will fail until ffmpeg is installed."

    $env:CONNECT_ALLOW_SYSTEM_FFMPEG = "1"

  }

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



# Production default: DXGI + libx264 ffmpeg (see internal/agent/encoder.go).
Remove-Item Env:CONNECT_ENCODER_CODEC -ErrorAction SilentlyContinue
Remove-Item Env:CONNECT_ENCODER_GDIGRAB -ErrorAction SilentlyContinue
Remove-Item Env:CONNECT_ENCODER_REPROBE -ErrorAction SilentlyContinue
Remove-Item (Join-Path $configDir "encoder.json") -Force -ErrorAction SilentlyContinue

Write-Host "Encoder pipeline: DXGI + libx264 (gdigrab fallback); requires bin\ffmpeg.exe"



Stop-Process -Name connect-agent -Force -ErrorAction SilentlyContinue

Start-Sleep -Seconds 2

Start-Process -FilePath $Agent -ArgumentList @("-server", $Server) -WindowStyle Hidden

Write-Host "connect-agent started -> $Server"

Write-Host ("Log file: {0}" -f (Join-Path $configDir "agent.log"))

Write-Host "Run viewer session, then: .\deploy\check-phase-a.ps1"

