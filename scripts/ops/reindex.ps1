# Rebuild OpenSearch/Qdrant projection from Postgres. Requires .env / SHARED_MEMORY_ENV.
param(
    [string]$EnvFile = ".env"
)
$ErrorActionPreference = "Stop"
if ($EnvFile) { $env:SHARED_MEMORY_ENV = $EnvFile }
Write-Host "Running worker -reindex ..."
go run ./cmd/worker -reindex
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Reindex complete."
