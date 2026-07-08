# Parse agent.log and report Phase A pass/fail for the latest viewer session.
param(
  [string]$LogPath = (Join-Path $env:LOCALAPPDATA "Connect\agent.log"),
  [int]$MinUptimeSec = 60,
  [int]$MaxFirstFrameMs = 3000,
  [double]$MinSendFps = 15.0,
  [int]$SoakMinutes = 0
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $LogPath)) {
  Write-Host "FAIL  log not found: $LogPath"
  Write-Host "      Run a viewer session first (.\deploy\start.ps1)"
  exit 1
}

$lines = Get-Content $LogPath -ErrorAction Stop

$lastStartIdx = -1
for ($i = $lines.Count - 1; $i -ge 0; $i--) {
  if ($lines[$i] -match 'connect-agent starting') {
    $lastStartIdx = $i
    break
  }
}

$session = $null
$sessionStartIdx = -1
for ($i = $lines.Count - 1; $i -ge 0; $i--) {
  if ($lastStartIdx -ge 0 -and $i -lt $lastStartIdx) { break }
  if ($lines[$i] -match 'session ([A-Z0-9]+) requested') {
    $session = $Matches[1]
    $sessionStartIdx = $i
    break
  }
}

if (-not $session) {
  Write-Host "FAIL  no viewer session in log since last agent start"
  Write-Host "      Connect from http://127.0.0.1:8787/dashboard/ and retry"
  exit 1
}

$slice = $lines[$sessionStartIdx..($lines.Count - 1)]

if ($SoakMinutes -gt 0) {
  $MinUptimeSec = [math]::Max($MinUptimeSec, $SoakMinutes * 60)
}

$connected = $false
$firstFrameMs = $null
$bestFps = $null
$bestUptime = 0
$lastFps = $null
$lastUptime = 0
$stalled = $false
$encoderReadyMs = $null
$pipelineOk = $false
$dtsErrors = 0
$encoderEOF = $false
$pipelineLine = $null
$summaryLine = $null

foreach ($line in $slice) {
  if ($line -match "WebRTC state connected \(session $session\)") { $connected = $true }
  if ($line -match "encoder .+ \(ready_ms=(\d+)\)") { $encoderReadyMs = [int]$Matches[1] }
  if ($line -match "perf session $session first_frame_ms=(\d+)") { $firstFrameMs = [int]$Matches[1] }
  if ($line -match "perf session $session samples=(\d+) send_fps=([\d.]+) uptime_s=([\d.]+)") {
    $fps = [double]$Matches[2]
    $uptime = [double]$Matches[3]
    $lastFps = $fps
    $lastUptime = $uptime
    if ($uptime -ge $MinUptimeSec -and ($null -eq $bestFps -or $fps -gt $bestFps)) {
      $bestFps = $fps
      $bestUptime = $uptime
    }
  }
  if ($line -match "video stalled \(session $session\)") { $stalled = $true }
  if ($line -match "agent: pipeline capture=") { $pipelineLine = $line }
  if ($line -match "agent: session $session summary ") { $summaryLine = $line }
  if ($line -match 'non monotonically increasing dts') { $dtsErrors++ }
  if ($line -match "encoder EOF \(session $session\)") { $encoderEOF = $true }
}

# Pipeline check: structured pipeline line since last agent start
$agentSlice = $lines
if ($lastStartIdx -ge 0) {
  $agentSlice = $lines[$lastStartIdx..($lines.Count - 1)]
}
foreach ($line in $agentSlice) {
  if ($line -match 'agent: pipeline capture=(\w+) codec=(\S+)') {
    $pipelineOk = $true
    break
  }
}

$fpsForGate = if ($null -ne $bestFps) { $bestFps } else { $lastFps }
$uptimeForGate = if ($null -ne $bestFps) { $bestUptime } else { $lastUptime }

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

if ($null -ne $fpsForGate -and $uptimeForGate -ge $MinUptimeSec -and $fpsForGate -ge $MinSendFps) {
  $results += [pscustomobject]@{ Check = "send_fps >= $MinSendFps (after ${MinUptimeSec}s)"; Status = "PASS"; Detail = "$([math]::Round($fpsForGate, 1)) fps @ ${uptimeForGate}s uptime" }
} elseif ($null -ne $fpsForGate -and $uptimeForGate -ge $MinUptimeSec) {
  $results += [pscustomobject]@{ Check = "send_fps >= $MinSendFps (after ${MinUptimeSec}s)"; Status = "FAIL"; Detail = "$([math]::Round($fpsForGate, 1)) fps @ ${uptimeForGate}s uptime" }
} else {
  $results += [pscustomobject]@{ Check = "send_fps >= $MinSendFps (after ${MinUptimeSec}s)"; Status = "FAIL"; Detail = "keep viewer open >= ${MinUptimeSec}s" }
}

if (-not $stalled) {
  $results += [pscustomobject]@{ Check = "no video stalled"; Status = "PASS"; Detail = "none" }
} else {
  $results += [pscustomobject]@{ Check = "no video stalled"; Status = "FAIL"; Detail = "session ended on stall" }
}

if ($pipelineOk) {
  $results += [pscustomobject]@{ Check = "live-probed pipeline"; Status = "PASS"; Detail = "probe or pipeline log present" }
} else {
  $results += [pscustomobject]@{ Check = "live-probed pipeline"; Status = "FAIL"; Detail = "no pipeline/probe log (blind codec?)" }
}

if ($dtsErrors -eq 0) {
  $results += [pscustomobject]@{ Check = "no DTS errors"; Status = "PASS"; Detail = "none" }
} else {
  $results += [pscustomobject]@{ Check = "no DTS errors"; Status = "FAIL"; Detail = "$dtsErrors occurrence(s)" }
}

if (-not $encoderEOF) {
  $results += [pscustomobject]@{ Check = "no encoder EOF"; Status = "PASS"; Detail = "none" }
} else {
  $results += [pscustomobject]@{ Check = "no encoder EOF"; Status = "FAIL"; Detail = "encoder died mid-session" }
}

Write-Host ""
$title = if ($SoakMinutes -gt 0) { "Phase A soak ($SoakMinutes min) - session $session" } else { "Phase A check - session $session" }
Write-Host $title
Write-Host "Log: $LogPath"
if ($null -ne $encoderReadyMs) {
  Write-Host "Encoder ready: ${encoderReadyMs} ms"
}
if ($pipelineLine) {
  Write-Host "Pipeline: $pipelineLine"
}
if ($summaryLine) {
  Write-Host "Summary: $summaryLine"
}
Write-Host ""

$results | ForEach-Object {
  $mark = if ($_.Status -eq "PASS") { "OK  " } else { "FAIL" }
  Write-Host "$mark  $($_.Check)  -  $($_.Detail)"
}

$failed = @($results | Where-Object { $_.Status -eq "FAIL" }).Count
Write-Host ""
if ($failed -eq 0) {
  if ($SoakMinutes -gt 0) {
    Write-Host "RESULT: Phase A SOAK PASSED - safe for release gate"
  } else {
    Write-Host "RESULT: Phase A PASSED - safe to move to Phase B (phone on Wi-Fi)"
  }
  Write-Host "See docs/RELEASE-GATE.md"
  exit 0
}

Write-Host "RESULT: Phase A FAILED ($failed check(s)) - stay on localhost, fix perf before firewall/Render"
Write-Host "See docs/PHASE-A.md and docs/RELEASE-GATE.md"
exit 1
