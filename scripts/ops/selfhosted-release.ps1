# Self-hosted release checklist (no GitHub required).
# Usage: powershell -File scripts/ops/selfhosted-release.ps1 [-Api http://127.0.0.1:18080] [-Reindex] [-Smoke] [-Tag v0.x.x]
param(
    [string]$Api = "http://127.0.0.1:18080",
    [switch]$Reindex,
    [switch]$Smoke,
    [string]$Tag = ""
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "../..")

Write-Host "1) Build API + worker" -ForegroundColor Cyan
go build -o bin/shared-memory-api.exe ./cmd/api
go build -o bin/shared-memory-worker.exe ./cmd/worker

Write-Host "2) Web console" -ForegroundColor Cyan
Push-Location web
npm run sync-docs
npm run build
Pop-Location

if ($env:POSTGRES_DSN) {
    Write-Host "3) Verify migrations (0011-0013)" -ForegroundColor Cyan
    psql $env:POSTGRES_DSN -f scripts/ops/verify-migrations.sql
} else {
    Write-Host "3) Skip migration SQL (set POSTGRES_DSN to verify)" -ForegroundColor Yellow
}

if ($Reindex) {
    Write-Host "4) Reindex" -ForegroundColor Cyan
    & "$PSScriptRoot/reindex.ps1"
}

if ($Smoke) {
    Write-Host "5) Smoke" -ForegroundColor Cyan
    & (Join-Path $PSScriptRoot "../e2e-smoke.ps1") -Api $Api
}

if ($Tag) {
    Write-Host "6) Git tag $Tag" -ForegroundColor Cyan
    git tag -a $Tag -m "Self-hosted release $Tag"
    Write-Host "Tagged. Push/tag locally as you prefer (no GitHub assumed)."
}

Write-Host "Done." -ForegroundColor Green
