#Requires -Version 5.1
param(
  [string]$ApiUrl,
  [string]$ApiKey,
  [string]$ConnectionId,
  [string]$InstallAgent
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Server replaces __SM_*__ once at connection-create time.
if ([string]::IsNullOrWhiteSpace($ApiUrl)) { $ApiUrl = '__SM_API_URL__' }
if ([string]::IsNullOrWhiteSpace($ApiKey)) { $ApiKey = '__SM_API_KEY__' }
if ([string]::IsNullOrWhiteSpace($ConnectionId)) { $ConnectionId = '__SM_CONNECTION_ID__' }
if ([string]::IsNullOrWhiteSpace($InstallAgent)) { $InstallAgent = '__SM_INSTALL_AGENT__' }

Set-Location $PSScriptRoot

function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }

Write-Step "Shared Memory setup (install __SM_INSTALL_AGENT__)"

$credDir = Join-Path $env:USERPROFILE '.shared-memory'
if (-not (Test-Path -LiteralPath $credDir)) {
  New-Item -ItemType Directory -Path $credDir -Force | Out-Null
}
$credPath = Join-Path $credDir 'credentials.json'
$credJson = (@{
  api_url       = $ApiUrl
  api_key       = $ApiKey
  connection_id = $ConnectionId
} | ConvertTo-Json)
$utf8 = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($credPath, $credJson + "`n", $utf8)

$smctl = Join-Path $PSScriptRoot 'smctl.exe'
if (-not (Test-Path -LiteralPath $smctl)) {
  throw "Missing smctl.exe next to this script ($smctl)"
}

$smhook = Join-Path $PSScriptRoot 'smhook.exe'
$mcp = Join-Path $PSScriptRoot 'shared-memory-mcp.exe'

$installArgs = @('install', $InstallAgent)
if ((Test-Path -LiteralPath $smhook) -and (Test-Path -LiteralPath $mcp)) {
  $installArgs += @('--smhook', $smhook, '--mcp', $mcp)
} elseif (Test-Path -LiteralPath $mcp) {
  # Doubao (and similar): MCP only, no hook binary
  $installArgs += @('--mcp', $mcp)
}

Write-Step "smctl install __SM_INSTALL_AGENT__"
& $smctl @installArgs
if ($LASTEXITCODE -ne 0) {
  throw "smctl install failed with exit code $LASTEXITCODE"
}
Write-Host "Done." -ForegroundColor Green
