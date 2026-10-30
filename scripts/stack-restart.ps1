# Restart Shared Memory stack
# Usage: powershell -File scripts/stack-restart.ps1
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $here "stack-stop.ps1")
& (Join-Path $here "stack-start.ps1") @args
