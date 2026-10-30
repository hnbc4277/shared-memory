# Stop Shared Memory stack (containers + web preview)
# Usage: powershell -File scripts/stack-stop.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

$pidFile = Join-Path $root ".tmp\web-preview.pid"
if (Test-Path $pidFile) {
    $old = Get-Content $pidFile -ErrorAction SilentlyContinue
    if ($old) {
        Write-Host "==> Stop web preview (pid $old)" -ForegroundColor Cyan
        Stop-Process -Id ([int]$old) -Force -ErrorAction SilentlyContinue
        # also kill orphaned vite on 15173 if any
        Get-NetTCPConnection -LocalPort 15173 -ErrorAction SilentlyContinue |
            ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }
    }
    Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
}

Write-Host "==> Stop Docker services (data on disk is kept)" -ForegroundColor Cyan
docker compose --profile app down
Write-Host "Stopped." -ForegroundColor Green
