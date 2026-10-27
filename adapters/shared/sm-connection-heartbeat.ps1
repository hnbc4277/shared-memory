#Requires -Version 5.1
# Shared Memory connection keepalive (install-side).
# Reads SHARED_MEMORY_API_URL / SHARED_MEMORY_API_KEY / SHARED_MEMORY_CONNECTION_ID.
# Optional SHARED_MEMORY_HEARTBEAT_INTERVAL (seconds, default 30).
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Continue'
$ApiUrl = $env:SHARED_MEMORY_API_URL
if ([string]::IsNullOrWhiteSpace($ApiUrl)) { $ApiUrl = 'http://127.0.0.1:18765' }
$ApiKey = $env:SHARED_MEMORY_API_KEY
$ConnectionId = $env:SHARED_MEMORY_CONNECTION_ID
$Interval = 30
if ($env:SHARED_MEMORY_HEARTBEAT_INTERVAL) {
  $parsed = 0
  if ([int]::TryParse($env:SHARED_MEMORY_HEARTBEAT_INTERVAL, [ref]$parsed) -and $parsed -gt 0) {
    $Interval = $parsed
  }
}
if ([string]::IsNullOrWhiteSpace($ApiKey)) {
  Write-Error 'sm-connection-heartbeat: SHARED_MEMORY_API_KEY is required'
  exit 1
}
$ApiUrl = $ApiUrl.TrimEnd('/')
$body = @{ client = 'setup' }
if (-not [string]::IsNullOrWhiteSpace($ConnectionId)) {
  $body.connection_id = $ConnectionId
}
while ($true) {
  try {
    Invoke-RestMethod -Method Post -Uri "$ApiUrl/v1/agents/connections/heartbeat" -Headers @{ Authorization = "Bearer $ApiKey" } -ContentType 'application/json' -Body ($body | ConvertTo-Json)
  } catch {
    Write-Warning $_.Exception.Message
  }
  Start-Sleep -Seconds $Interval
}
