# Copy adapter/spec markdown into web/public for console doc links (no VITE_REPO_DOCS_BASE).
# Resolves docs/public vs docs/private by folder *contents* so the script stays ASCII-safe on Windows PowerShell 5.1.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$pub = Join-Path $root "web\public"
$docsRoot = Join-Path $root "docs"

function Find-DocsBucket([string]$markerRel) {
    foreach ($dir in Get-ChildItem -LiteralPath $docsRoot -Directory -ErrorAction SilentlyContinue) {
        $candidate = Join-Path $dir.FullName $markerRel
        if (Test-Path -LiteralPath $candidate) { return $dir.FullName }
    }
    return $null
}

$privateDocs = Find-DocsBucket "superpowers\specs"
if (-not $privateDocs) {
    throw "Cannot find docs bucket containing superpowers/specs (expected under docs/)."
}

$maps = @(
    @{ Src = "adapters\hermes\README.md"; Dst = "adapters\hermes\README.md" },
    @{ Src = "adapters\openclaw\README.md"; Dst = "adapters\openclaw\README.md" },
    @{ Src = "adapters\deepseek-harness\README.md"; Dst = "adapters\deepseek-harness\README.md" },
    @{
        Src = (Join-Path $privateDocs "superpowers\specs\2026-09-30-shared-memory-design.md")
        Dst = "docs\superpowers\specs\2026-09-30-shared-memory-design.md"
        AbsoluteSrc = $true
    },
    @{
        Src = (Join-Path $privateDocs "superpowers\specs\2026-10-12-agent-connections-design.md")
        Dst = "docs\superpowers\specs\2026-10-12-agent-connections-design.md"
        AbsoluteSrc = $true
    }
)

foreach ($m in $maps) {
    $src = if ($m.AbsoluteSrc) { $m.Src } else { Join-Path $root $m.Src }
    $dst = Join-Path $pub $m.Dst
    $dir = Split-Path $dst -Parent
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    if (-not (Test-Path -LiteralPath $src)) { throw "missing source: $src" }
    Copy-Item -LiteralPath $src -Destination $dst -Force
    Write-Host "synced $($m.Dst)"
}
