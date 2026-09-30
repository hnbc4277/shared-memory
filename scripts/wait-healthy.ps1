$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$TimeoutSec = if ($env:WAIT_TIMEOUT) { [int]$env:WAIT_TIMEOUT } else { 180 }
$Services = @("postgres", "redis", "minio", "opensearch", "qdrant")
$Deadline = (Get-Date).AddSeconds($TimeoutSec)

while ((Get-Date) -lt $Deadline) {
    $allOk = $true
    foreach ($svc in $Services) {
        $cid = docker compose ps -q $svc 2>$null
        if (-not $cid) {
            $allOk = $false
            break
        }
        $status = docker inspect -f "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}" $cid 2>$null
        if ($status -ne "healthy" -and $status -ne "running") {
            $allOk = $false
            break
        }
    }
    if ($allOk) {
        Write-Host "All services ready."
        exit 0
    }
    Start-Sleep -Seconds 3
}

Write-Error "Timed out after ${TimeoutSec}s waiting for services."
docker compose ps
exit 1
