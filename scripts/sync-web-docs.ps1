# Copy adapter READMEs into web/public for console doc links (no VITE_REPO_DOCS_BASE).
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$pub = Join-Path $root "web\public"

$maps = @(
    @{ Src = "adapters\hermes\README.md"; Dst = "adapters\hermes\README.md" },
    @{ Src = "adapters\openclaw\README.md"; Dst = "adapters\openclaw\README.md" },
    @{ Src = "adapters\deepseek-harness\README.md"; Dst = "adapters\deepseek-harness\README.md" },
    @{ Src = "adapters\doubao\README.md"; Dst = "adapters\doubao\README.md" }
)

foreach ($m in $maps) {
    $src = Join-Path $root $m.Src
    $dst = Join-Path $pub $m.Dst
    $dir = Split-Path $dst -Parent
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    if (-not (Test-Path -LiteralPath $src)) { throw "missing source: $src" }
    Copy-Item -LiteralPath $src -Destination $dst -Force
    Write-Host "synced $($m.Dst)"
}
