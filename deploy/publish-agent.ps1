# Build connect-agent.zip + BlueConnect-Setup.exe for Host install links.
# Requires Windows + CGO/gcc + ffmpeg.exe.
#
#   .\deploy\publish-agent.ps1
#   .\deploy\publish-agent.ps1 -OutZip .\agent.zip
#
# Outputs (default):
#   data\agent\agent.zip
#   data\agent\BlueConnect-Setup.exe

param(
  [string]$OutZip = "",
  [switch]$SkipBuild,
  [string]$FFmpegPath = "",
  # Authenticode thumbprint (or set CONNECT_CODE_SIGN_THUMBPRINT). Required to reduce SmartScreen warnings.
  [string]$SignThumbprint = $env:CONNECT_CODE_SIGN_THUMBPRINT,
  [string]$SignTimestampUrl = "http://timestamp.digicert.com"
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

function Find-SignTool {
  $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $roots = @(
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
    "${env:ProgramFiles}\Windows Kits\10\bin"
  )
  foreach ($root in $roots) {
    if (-not (Test-Path -LiteralPath $root)) { continue }
    $hit = Get-ChildItem -Path $root -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
      Sort-Object FullName -Descending |
      Select-Object -First 1 -ExpandProperty FullName
    if ($hit) { return $hit }
  }
  return $null
}

function Sign-ConnectBinary {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Thumbprint,
    [string]$TimestampUrl
  )
  $tool = Find-SignTool
  if (-not $tool) {
    throw "signtool.exe not found. Install Windows SDK Signing Tools, or sign manually."
  }
  $tp = ($Thumbprint -replace "\s", "").ToUpperInvariant()
  Write-Host ("  signing: {0}" -f $Path)
  & $tool sign /fd SHA256 /td SHA256 /tr $TimestampUrl /sha1 $tp $Path
  if ($LASTEXITCODE -ne 0) {
    throw ("signtool failed for {0} (exit {1})" -f $Path, $LASTEXITCODE)
  }
}

Write-Host "Connect - publish agent package"

$OutDir = Join-Path $Root "data\agent"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

if (-not $SkipBuild) {
  Write-Host "Building connect-agent.exe (CGO)..."
  $env:CGO_ENABLED = "1"
  $outExe = Join-Path $Root "connect-agent.exe"
  & go build -trimpath "-ldflags=-s -w -H=windowsgui" -o $outExe ./cmd/connect-agent
  if ($LASTEXITCODE -ne 0) {
    throw "go build connect-agent failed"
  }

  Write-Host "Building BlueConnect-Setup.exe..."
  $setupOut = Join-Path $OutDir "BlueConnect-Setup.exe"
  & go build -trimpath "-ldflags=-s -w -H=windowsgui" -o $setupOut ./cmd/connect-setup
  if ($LASTEXITCODE -ne 0) {
    throw "go build connect-setup failed"
  }
  Write-Host "  setup: $setupOut"
} else {
  $setupSrc = Join-Path $Root "BlueConnect-Setup.exe"
  $setupOut = Join-Path $OutDir "BlueConnect-Setup.exe"
  if (Test-Path -LiteralPath $setupSrc) {
    Copy-Item -LiteralPath $setupSrc -Destination $setupOut -Force
  } elseif (-not (Test-Path -LiteralPath $setupOut)) {
    Write-Host "WARNING: BlueConnect-Setup.exe missing - build without -SkipBuild"
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

# Keep Setup.exe next to agent.zip (same CONNECT_AGENT_DIR on the server).
$setupFinal = Join-Path $OutDir "BlueConnect-Setup.exe"
if (Test-Path -LiteralPath $setupFinal) {
  $setupParent = Split-Path -Parent $OutZip
  if ($setupParent -and ((Resolve-Path $setupParent).Path -ne (Resolve-Path $OutDir).Path)) {
    Copy-Item -LiteralPath $setupFinal -Destination (Join-Path $setupParent "BlueConnect-Setup.exe") -Force
  }
}

if ($SignThumbprint) {
  Write-Host "Code-signing with thumbprint $SignThumbprint ..."
  Sign-ConnectBinary -Path $Exe -Thumbprint $SignThumbprint -TimestampUrl $SignTimestampUrl
  if (Test-Path -LiteralPath $setupFinal) {
    Sign-ConnectBinary -Path $setupFinal -Thumbprint $SignThumbprint -TimestampUrl $SignTimestampUrl
  }
  # Rebuild zip with signed agent.exe
  $Stage2 = Join-Path $env:TEMP ("connect-agent-pkg-signed-" + [guid]::NewGuid().ToString())
  $Stage2Bin = Join-Path $Stage2 "bin"
  New-Item -ItemType Directory -Force -Path $Stage2Bin | Out-Null
  Copy-Item -LiteralPath $Exe -Destination (Join-Path $Stage2 "connect-agent.exe") -Force
  Copy-Item -LiteralPath $Ff -Destination (Join-Path $Stage2Bin "ffmpeg.exe") -Force
  if (Test-Path -LiteralPath $OutZip) { Remove-Item -LiteralPath $OutZip -Force }
  Compress-Archive -Path (Join-Path $Stage2 "*") -DestinationPath $OutZip -Force
  Remove-Item -LiteralPath $Stage2 -Recurse -Force
  Write-Host "  signed package ready"
} else {
  Write-Host ""
  Write-Host "NOTE: binaries are UNSIGNED. Browsers/SmartScreen will warn on Download."
  Write-Host "      Buy an Authenticode code-signing cert, then re-run:"
  Write-Host "      .\deploy\publish-agent.ps1 -SignThumbprint YOUR_CERT_THUMBPRINT"
  Write-Host "      (or set CONNECT_CODE_SIGN_THUMBPRINT)"
}

$sizeMb = [math]::Round((Get-Item -LiteralPath $OutZip).Length / 1MB, 1)
Write-Host ('Wrote {0} ({1} MB)' -f $OutZip, $sizeMb)
if (Test-Path -LiteralPath $setupFinal) {
  $setupMb = [math]::Round((Get-Item -LiteralPath $setupFinal).Length / 1MB, 1)
  Write-Host ('Wrote {0} ({1} MB)' -f $setupFinal, $setupMb)
}
Write-Host ""
Write-Host "Next:"
Write-Host "  Local: restart connectd (serves /download/agent.zip + /download/setup.exe from data\agent)"
Write-Host "  VPS:   copy agent.zip AND BlueConnect-Setup.exe into the agent dir, or Admin-upload zip + scp Setup.exe"
Write-Host "  Hosts: open /install?code=... then Download BlueConnect (Setup.exe)"
