# Build connect-agent.zip for Host install links (/download/agent.zip).
# Requires Windows + CGO/gcc + ffmpeg.exe.
#
#   .\deploy\publish-agent.ps1
#   .\deploy\publish-agent.ps1 -OutZip .\agent.zip

param(
  [string]$OutZip = "",
  [switch]$SkipBuild,
  [string]$FFmpegPath = ""
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

function Find-FFmpeg {
  param([string]$Hint)
  if ($Hint -and (Test-Path -LiteralPath $Hint)) {
    return (Resolve-Path -LiteralPath $Hint).Path
  }
  $candidates = @(
    (Join-Path $Root "bin\ffmpeg.exe"),
    (Join-Path $Root "ffmpeg.exe"),
    (Join-Path $env:LOCALAPPDATA "Connect\bin\ffmpeg.exe"),
    (Join-Path $env:LOCALAPPDATA "Connect\ffmpeg.exe")
  )
  foreach ($p in $candidates) {
    if (Test-Path -LiteralPath $p) {
      return (Resolve-Path -LiteralPath $p).Path
    }
  }
  $wingetRoot = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
  if (Test-Path -LiteralPath $wingetRoot) {
    $winget = Get-ChildItem -Path $wingetRoot -Filter "ffmpeg.exe" -Recurse -ErrorAction SilentlyContinue |
      Select-Object -First 1 -ExpandProperty FullName
    if ($winget) { return $winget }
  }
  $sys = Get-Command "ffmpeg" -ErrorAction SilentlyContinue
  if ($sys) { return $sys.Source }
  return $null
}

Write-Host "Connect - publish agent package"

if (-not $SkipBuild) {
  Write-Host "Building connect-agent.exe (CGO)..."
  $env:CGO_ENABLED = "1"
  $outExe = Join-Path $Root "connect-agent.exe"
  & go build -trimpath "-ldflags=-s -w -H=windowsgui" -o $outExe ./cmd/connect-agent
  if ($LASTEXITCODE -ne 0) {
    throw "go build failed"
  }
}

$Exe = Join-Path $Root "connect-agent.exe"
if (-not (Test-Path -LiteralPath $Exe)) {
  throw "connect-agent.exe not found - run without -SkipBuild or build first"
}

$Ff = Find-FFmpeg -Hint $FFmpegPath
if (-not $Ff) {
  throw "ffmpeg.exe not found. Pass -FFmpegPath or place bin\ffmpeg.exe in the repo."
}
Write-Host "  ffmpeg: $Ff"

$Stage = Join-Path $env:TEMP ("connect-agent-pkg-" + [guid]::NewGuid().ToString())
$StageBin = Join-Path $Stage "bin"
New-Item -ItemType Directory -Force -Path $StageBin | Out-Null
Copy-Item -LiteralPath $Exe -Destination (Join-Path $Stage "connect-agent.exe") -Force
Copy-Item -LiteralPath $Ff -Destination (Join-Path $StageBin "ffmpeg.exe") -Force

if (-not $OutZip) {
  $OutDir = Join-Path $Root "data\agent"
  New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
  $OutZip = Join-Path $OutDir "agent.zip"
}

if (-not [System.IO.Path]::IsPathRooted($OutZip)) {
  $OutZip = Join-Path (Get-Location).Path $OutZip
}
$OutParent = Split-Path -Parent $OutZip
if ($OutParent) {
  New-Item -ItemType Directory -Force -Path $OutParent | Out-Null
}
if (Test-Path -LiteralPath $OutZip) {
  Remove-Item -LiteralPath $OutZip -Force
}

Compress-Archive -Path (Join-Path $Stage "*") -DestinationPath $OutZip -Force
Remove-Item -LiteralPath $Stage -Recurse -Force

$sizeMb = [math]::Round((Get-Item -LiteralPath $OutZip).Length / 1MB, 1)
Write-Host "Wrote $OutZip ($sizeMb MB)"
Write-Host ""
Write-Host "Next:"
Write-Host "  Local: restart connectd (serves /download/agent.zip from data\agent)"
Write-Host "  VPS:   scp `"$OutZip`" user@vps:~/connect/deploy/agent/agent.zip"
Write-Host "         then: docker compose up -d --force-recreate connectd"
