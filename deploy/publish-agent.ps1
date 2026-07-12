# Build connect-agent.zip + WorthyJoin-Setup.exe for Host install links.
# Requires Windows + CGO/gcc + ffmpeg.exe.
#
#   .\deploy\publish-agent.ps1
#   .\deploy\publish-agent.ps1 -OutZip .\agent.zip
#
# Outputs (default):
#   data\agent\agent.zip
#   data\agent\WorthyJoin-Setup.exe

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

  Write-Host "Building WorthyJoin-Host.exe (CGO)..."
  $hostOut = Join-Path $Root "WorthyJoin-Host.exe"
  Write-Host "  embedding Host app icon..."
  & go run ./cmd/connect-host/genicon.go
  if ($LASTEXITCODE -ne 0) { throw "genicon failed" }
  & go run ./cmd/connect-host/genrsrc.go
  if ($LASTEXITCODE -ne 0) { throw "genrsrc failed" }
  & go build -trimpath "-ldflags=-s -w -H=windowsgui" -o $hostOut ./cmd/connect-host
  if ($LASTEXITCODE -ne 0) {
    throw "go build connect-host failed"
  }

  Write-Host "Building WorthyJoin-Setup.exe..."
  $setupOut = Join-Path $OutDir "WorthyJoin-Setup.exe"
  & go build -trimpath "-ldflags=-s -w -H=windowsgui" -o $setupOut ./cmd/connect-setup
  if ($LASTEXITCODE -ne 0) {
    throw "go build connect-setup failed"
  }
  Write-Host "  setup: $setupOut"
} else {
  $setupSrc = Join-Path $Root "WorthyJoin-Setup.exe"
  $setupOut = Join-Path $OutDir "WorthyJoin-Setup.exe"
  if (Test-Path -LiteralPath $setupSrc) {
    Copy-Item -LiteralPath $setupSrc -Destination $setupOut -Force
  } elseif (-not (Test-Path -LiteralPath $setupOut)) {
    Write-Host "WARNING: WorthyJoin-Setup.exe missing - build without -SkipBuild"
  }
}

$Exe = Join-Path $Root "connect-agent.exe"
if (-not (Test-Path -LiteralPath $Exe)) {
  throw "connect-agent.exe not found - run without -SkipBuild or build first"
}
$HostExe = Join-Path $Root "WorthyJoin-Host.exe"
if (-not (Test-Path -LiteralPath $HostExe)) {
  throw "WorthyJoin-Host.exe not found - run without -SkipBuild or build first"
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
Copy-Item -LiteralPath $HostExe -Destination (Join-Path $Stage "WorthyJoin-Host.exe") -Force
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
$setupFinal = Join-Path $OutDir "WorthyJoin-Setup.exe"
if (Test-Path -LiteralPath $setupFinal) {
  $setupParent = Split-Path -Parent $OutZip
  if ($setupParent -and ((Resolve-Path $setupParent).Path -ne (Resolve-Path $OutDir).Path)) {
    Copy-Item -LiteralPath $setupFinal -Destination (Join-Path $setupParent "WorthyJoin-Setup.exe") -Force
  }
}

if ($SignThumbprint) {
  Write-Host "Code-signing with thumbprint $SignThumbprint ..."
  Sign-ConnectBinary -Path $Exe -Thumbprint $SignThumbprint -TimestampUrl $SignTimestampUrl
  Sign-ConnectBinary -Path $HostExe -Thumbprint $SignThumbprint -TimestampUrl $SignTimestampUrl
  if (Test-Path -LiteralPath $setupFinal) {
    Sign-ConnectBinary -Path $setupFinal -Thumbprint $SignThumbprint -TimestampUrl $SignTimestampUrl
  }
  # Rebuild zip with signed binaries
  $Stage2 = Join-Path $env:TEMP ("connect-agent-pkg-signed-" + [guid]::NewGuid().ToString())
  $Stage2Bin = Join-Path $Stage2 "bin"
  New-Item -ItemType Directory -Force -Path $Stage2Bin | Out-Null
  Copy-Item -LiteralPath $Exe -Destination (Join-Path $Stage2 "connect-agent.exe") -Force
  Copy-Item -LiteralPath $HostExe -Destination (Join-Path $Stage2 "WorthyJoin-Host.exe") -Force
  Copy-Item -LiteralPath $Ff -Destination (Join-Path $Stage2Bin "ffmpeg.exe") -Force
  if (Test-Path -LiteralPath $OutZip) { Remove-Item -LiteralPath $OutZip -Force }
  Compress-Archive -Path (Join-Path $Stage2 "*") -DestinationPath $OutZip -Force
  Remove-Item -LiteralPath $Stage2 -Recurse -Force
  Write-Host "  signed package ready"
} else {
  Write-Host ""
  Write-Host "NOTE: binaries are UNSIGNED. SmartScreen may warn when hosts run the installer."
  Write-Host "      Browser download uses WorthyJoin-Install.zip (milder than raw .exe)."
  Write-Host "      Buy an Authenticode code-signing cert, then re-run:"
  Write-Host "      .\deploy\publish-agent.ps1 -SignThumbprint YOUR_CERT_THUMBPRINT"
  Write-Host "      (or set CONNECT_CODE_SIGN_THUMBPRINT)"
}

# Host-facing zip: Extract All -> double-click "Install WorthyJoin.exe" (Setup + agent.zip bundled).
$installZip = Join-Path $OutDir "WorthyJoin-Install.zip"
if ((Test-Path -LiteralPath $setupFinal) -and (Test-Path -LiteralPath $OutZip)) {
  $stageInstall = Join-Path $env:TEMP ("worthyjoin-install-" + [guid]::NewGuid().ToString())
  New-Item -ItemType Directory -Force -Path $stageInstall | Out-Null
  Copy-Item -LiteralPath $setupFinal -Destination (Join-Path $stageInstall "Install WorthyJoin.exe") -Force
  Copy-Item -LiteralPath $OutZip -Destination (Join-Path $stageInstall "agent.zip") -Force
  $readme = @"
WorthyJoin — install on this PC

1. Extract this whole folder (right-click the zip → Extract All).
2. Open the extracted folder.
3. Double-click: Install WorthyJoin.exe
4. Paste the enrollment code from your tech (if asked), then click Install.
5. If Windows SmartScreen appears: More info → Run anyway.
6. If Windows asks for permission (UAC): click Yes — that installs the host service.
7. Windows Defender may scan once; that is normal. WorthyJoin only allows its own folder under AppData\Local\Connect — it does not turn Defender off.

You can delete this folder after install finishes.
"@
  Set-Content -LiteralPath (Join-Path $stageInstall "README.txt") -Value $readme -Encoding UTF8
  if (Test-Path -LiteralPath $installZip) { Remove-Item -LiteralPath $installZip -Force }
  Compress-Archive -Path (Join-Path $stageInstall "*") -DestinationPath $installZip -Force
  Remove-Item -LiteralPath $stageInstall -Recurse -Force
  Write-Host ('Wrote {0} (host download)' -f $installZip)
} else {
  Write-Host "WARNING: WorthyJoin-Install.zip skipped (need Setup.exe + agent.zip)"
}

$sizeMb = [math]::Round((Get-Item -LiteralPath $OutZip).Length / 1MB, 1)
Write-Host ('Wrote {0} ({1} MB)' -f $OutZip, $sizeMb)
if (Test-Path -LiteralPath $setupFinal) {
  $setupMb = [math]::Round((Get-Item -LiteralPath $setupFinal).Length / 1MB, 1)
  Write-Host ('Wrote {0} ({1} MB)' -f $setupFinal, $setupMb)
  # Compat alias for older VPS copies / docs
  Copy-Item -LiteralPath $setupFinal -Destination (Join-Path $OutDir "BlueConnect-Setup.exe") -Force
}
Write-Host ""
Write-Host "Next:"
Write-Host "  Local: restart connectd (serves install zip + agent.zip from data\agent)"
Write-Host "  VPS:   copy WorthyJoin-Install.zip, agent.zip, WorthyJoin-Setup.exe into the agent dir"
Write-Host "  Hosts: /install?code=... → Download WorthyJoin (zip) → Extract All → Install WorthyJoin.exe"
