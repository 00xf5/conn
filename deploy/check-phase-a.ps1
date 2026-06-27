# Parse agent.log and report Phase A pass/fail for the latest viewer session.
param(
  [string]$LogPath = (Join-Path $env:LOCALAPPDATA "Connect\agent.log"),
  [int]$MinUptimeSec = 60,
  [int]$MaxFirstFrameMs = 3000,
  [double]$MinSendFps = 15.0
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $LogPath)) {
  Write-Host "FAIL  log not found: $LogPath"
  Write-Host "      Run a viewer session first (.\deploy\start.ps1)"
  exit 1
}

$lines = Get-Content $LogPath -ErrorAction Stop

function Get-SessionCode {
  param([string]$Line)
  if ($Line -match 'session ([A-Z0-9]+) requested') { return $Matches[1] }
  if ($Line -match 'session ([A-Z0-9]+)\)') { return $Matches[1] }
  return $null
}

$session = $null
$sessionStartIdx = -1
for ($i = $lines.Count - 1; $i -ge 0; $i--) {
  if ($lines[$i] -match 'session ([A-Z0-9]+) requested') {
    $session = $Matches[1]
    $sessionStartIdx = $i
    break
  }
}

if (-not $session) {
  Write-Host "FAIL  no viewer session in log"
  Write-Host "      Connect from http://127.0.0.1:8787/dashboard/ and retry"
  exit 1
}

$slice = $lines[$sessionStartIdx..($lines.Count - 1)]

$connected = $false
$firstFrameMs = $null
$bestFps = $null
$bestUptime = 0
$stalled = $false
$encoderReadyMs = $null

foreach ($line in $slice) {
  if ($line -match "WebRTC state connected \(session $session\)") { $connected = $true }
  if ($line -match "encoder .+ \(ready_ms=(\d+)\)") { $encoderReadyMs = [int]$Matches[1] }
  if ($line -match "perf session $session first_frame_ms=(\d+)") { $firstFrameMs = [int]$Matches[1] }
  if ($line -match "perf session $session samples=(\d+) send_fps=([\d.]+) uptime_s=([\d.]+)") {
    $fps = [double]$Matches[2]
    $uptime = [double]$Matches[3]
    if ($uptime -ge $MinUptimeSec -and ($null -eq $bestFps -or $fps -lt $bestFps)) {
      $bestFps = $fps
      $bestUptime = $uptime
    }
  }
  if ($line -match "video stalled \(session $session\)") { $stalled = $true }
}

$results = @()

if ($connected) {
  $results += [pscustomobject]@{ Check = "WebRTC connected"; Status = "PASS"; Detail = "session $session" }
} else {
  $results += [pscustomobject]@{ Check = "WebRTC connected"; Status = "FAIL"; Detail = "no connected state for $session" }
}

if ($null -ne $firstFrameMs -and $firstFrameMs -lt $MaxFirstFrameMs) {
  $results += [pscustomobject]@{ Check = "first_frame_ms < $MaxFirstFrameMs"; Status = "PASS"; Detail = "$firstFrameMs ms" }
} elseif ($null -ne $firstFrameMs) {
  $results += [pscustomobject]@{ Check = "first_frame_ms < $MaxFirstFrameMs"; Status = "FAIL"; Detail = "$firstFrameMs ms" }
} else {
  $results += [pscustomobject]@{ Check = "first_frame_ms < $MaxFirstFrameMs"; Status = "FAIL"; Detail = "missing (no video sent?)" }
}

if ($null -ne $bestFps -and $bestFps -ge $MinSendFps) {
  $results += [pscustomobject]@{ Check = "send_fps >= $MinSendFps (after ${MinUptimeSec}s)"; Status = "PASS"; Detail = "$([math]::Round($bestFps, 1)) fps @ ${bestUptime}s uptime" }
} elseif ($null -ne $bestFps) {
  $results += [pscustomobject]@{ Check = "send_fps >= $MinSendFps (after ${MinUptimeSec}s)"; Status = "FAIL"; Detail = "$([math]::Round($bestFps, 1)) fps @ ${bestUptime}s uptime" }
} else {
  $results += [pscustomobject]@{ Check = "send_fps >= $MinSendFps (after ${MinUptimeSec}s)"; Status = "FAIL"; Detail = "keep viewer open >= ${MinUptimeSec}s" }
}

if (-not $stalled) {
  $results += [pscustomobject]@{ Check = "no video stalled"; Status = "PASS"; Detail = "none" }
} else {
  $results += [pscustomobject]@{ Check = "no video stalled"; Status = "FAIL"; Detail = "session ended on stall" }
}

Write-Host ""
Write-Host "Phase A check — session $session"
Write-Host "Log: $LogPath"
if ($null -ne $encoderReadyMs) {
  Write-Host "Encoder ready: ${encoderReadyMs} ms"
}
Write-Host ""

$results | ForEach-Object {
  $mark = if ($_.Status -eq "PASS") { "OK  " } else { "FAIL" }
  Write-Host "$mark  $($_.Check)  —  $($_.Detail)"
}

$failed = @($results | Where-Object { $_.Status -eq "FAIL" }).Count
Write-Host ""
if ($failed -eq 0) {
  Write-Host "RESULT: Phase A PASSED — safe to move to Phase B (phone on Wi-Fi)"
  Write-Host "See docs/PHASE-A.md"
  exit 0
}

Write-Host "RESULT: Phase A FAILED ($failed check(s)) — stay on localhost, fix perf before firewall/Render"
Write-Host "See docs/PHASE-A.md"
exit 1
