# Verify VPS connectd + coturn from Windows (run on host PC after setup-vps.sh).
param(
  [string]$Domain = "blueconnect.online"
)

$ErrorActionPreference = "Stop"
$base = "https://$Domain"
$results = @()

function Add-Result($check, $status, $detail) {
  $script:results += [pscustomobject]@{ Check = $check; Status = $status; Detail = $detail }
}

try {
  $health = Invoke-RestMethod -Uri "$base/api/health" -TimeoutSec 15
  if ($health.ok) { Add-Result "health ok" "PASS" "agents=$($health.agents)" }
  else { Add-Result "health ok" "FAIL" "ok=false" }

  if ($health.turnExternal) { Add-Result "turnExternal" "PASS" "coturn configured on connectd" }
  else { Add-Result "turnExternal" "FAIL" "set CONNECT_TURN_URL + CONNECT_TURN_SECRET on VPS" }

  if ($health.iceServers -ge 2) { Add-Result "iceServers count" "PASS" "$($health.iceServers) servers" }
  else { Add-Result "iceServers count" "FAIL" "expected >= 2, got $($health.iceServers)" }
} catch {
  Add-Result "health fetch" "FAIL" $_.Exception.Message
}

try {
  $ice = Invoke-RestMethod -Uri "$base/api/ice" -TimeoutSec 15
  $turn = @($ice.iceServers | Where-Object { ($_.urls | Out-String) -match 'turn:' })
  if ($turn.Count -gt 0 -and $turn[0].username -and $turn[0].credential) {
    Add-Result "TURN credentials" "PASS" "username present"
  } else {
    Add-Result "TURN credentials" "FAIL" "missing turn entry or credentials"
  }
} catch {
  Add-Result "ice fetch" "FAIL" $_.Exception.Message
}

$results | Format-Table -AutoSize
$failed = @($results | Where-Object { $_.Status -eq "FAIL" })
if ($failed.Count -gt 0) {
  Write-Host "VPS check FAILED ($($failed.Count) issues)"
  exit 1
}
Write-Host "VPS check PASSED - point agent: .\deploy\start-vps-agent.ps1 -Server wss://$Domain/ws"
exit 0
