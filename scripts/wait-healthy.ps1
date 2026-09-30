$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$TimeoutSec = if ($env:WAIT_TIMEOUT) { [int]$env:WAIT_TIMEOUT } else { 180 }
$Services = @("postgres", "redis", "minio", "opensearch", "qdrant")
$Deadline = (Get-Date).AddSeconds($TimeoutSec)

function Test-ServiceHasHealthcheck {
    param([string]$ContainerId)
    $flag = docker inspect -f "{{if .Config.Healthcheck}}true{{else}}false{{end}}" $ContainerId 2>$null
    return $flag -eq "true"
}

while ((Get-Date) -lt $Deadline) {
    $allOk = $true
    foreach ($svc in $Services) {
        $cid = docker compose ps -q $svc 2>$null
        if (-not $cid) {
            $allOk = $false
            break
        }
        if (Test-ServiceHasHealthcheck $cid) {
            $status = docker inspect -f "{{.State.Health.Status}}" $cid 2>$null
            if ($status -ne "healthy") {
                $allOk = $false
                break
            }
        } else {
            $status = docker inspect -f "{{.State.Status}}" $cid 2>$null
            if ($status -ne "running") {
                $allOk = $false
                break
            }
        }
    }
    if ($allOk) {
        foreach ($svc in $Services) {
            $cid = docker compose ps -q $svc
            if (-not (Test-ServiceHasHealthcheck $cid)) {
                Write-Warning "service ${svc} has no healthcheck; accepting running but readiness is not health-gated"
            }
        }
        Write-Host "All services ready."
        exit 0
    }
    Start-Sleep -Seconds 3
}

Write-Error "Timed out after ${TimeoutSec}s waiting for services."
docker compose ps
exit 1
