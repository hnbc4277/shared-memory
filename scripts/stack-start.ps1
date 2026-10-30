# Start Shared Memory stack: DB/infra + API + Worker + Web console preview
# Usage: powershell -File scripts/stack-start.ps1
param(
    [switch]$SkipWeb,
    [switch]$NoBuild
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

Write-Host "==> Infra (Postgres / Redis / MinIO / OpenSearch / Qdrant / Embedder) + API + Worker" -ForegroundColor Cyan
$upArgs = @("--profile", "app", "up", "-d")
if (-not $NoBuild) { $upArgs = @("--profile", "app", "up", "-d", "--build") }
docker compose @upArgs
# Best-effort embedder (do not fail the whole stack)
docker compose up -d embedder-model embedder 2>$null | Out-Null

Write-Host "==> Wait API healthz" -ForegroundColor Cyan
$ok = $false
for ($i = 0; $i -lt 60; $i++) {
    try {
        $r = Invoke-WebRequest -Uri "http://127.0.0.1:18080/healthz" -UseBasicParsing -TimeoutSec 2
        if ($r.Content -match "ok") { $ok = $true; break }
    } catch {}
    Start-Sleep -Seconds 2
}
if (-not $ok) { throw "API healthz not ready on http://127.0.0.1:18080" }
Write-Host "    API ok: http://127.0.0.1:18080" -ForegroundColor Green

if (-not $SkipWeb) {
    Write-Host "==> Web console (vite preview :15173)" -ForegroundColor Cyan
    $web = Join-Path $root "web"
    $pidFile = Join-Path $root ".tmp\web-preview.pid"
    New-Item -ItemType Directory -Force -Path (Split-Path $pidFile) | Out-Null
    if (Test-Path $pidFile) {
        $old = Get-Content $pidFile -ErrorAction SilentlyContinue
        if ($old) { Stop-Process -Id ([int]$old) -Force -ErrorAction SilentlyContinue }
    }
    Push-Location $web
    if (-not (Test-Path "node_modules")) { npm install }
    npm run build
    $p = Start-Process -FilePath "npm.cmd" -ArgumentList @("run", "preview", "--", "--host", "127.0.0.1", "--port", "15173") `
        -WorkingDirectory $web -WindowStyle Hidden -PassThru
    Pop-Location
    Set-Content -Path $pidFile -Value $p.Id -Encoding ascii
    Start-Sleep -Seconds 2
    Write-Host "    Web: http://127.0.0.1:15173" -ForegroundColor Green
}

Write-Host ""
Write-Host "Started." -ForegroundColor Green
Write-Host "  API : http://127.0.0.1:18080/healthz"
if (-not $SkipWeb) { Write-Host "  Web : http://127.0.0.1:15173" }
Write-Host "  Data: F:\dockerData\sharedmemory\data (if override is present)"
docker compose --profile app ps
