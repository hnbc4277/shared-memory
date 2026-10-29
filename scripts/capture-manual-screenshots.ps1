# 从真实控制台页面生成操作手册配图（Playwright + 标注图例）
# 前置：API 已启动（默认 http://127.0.0.1:18080/healthz → ok）
# 用法：powershell -File scripts/capture-manual-screenshots.ps1
param(
    [string]$Api = "http://127.0.0.1:18080",
    [string]$WebPort = "15173"
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$web = Join-Path $root "web"
$shot = Join-Path $root "scripts\manual-screenshots"

try {
    $h = Invoke-WebRequest -Uri "$Api/healthz" -UseBasicParsing -TimeoutSec 5
    if ($h.Content -notmatch "ok") { throw "healthz 异常" }
} catch {
    Write-Host "API 未就绪。请先：docker compose --profile app up -d --build" -ForegroundColor Red
    exit 1
}

$env:MANUAL_API_URL = $Api
$env:MANUAL_WEB_URL = "http://127.0.0.1:$WebPort"

Push-Location $web
$env:VITE_MANUAL_SCREENSHOT = "1"
npm run build
Remove-Item Env:VITE_MANUAL_SCREENSHOT -ErrorAction SilentlyContinue
if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }

$npmCmd = $null
try { $npmCmd = (Get-Command npm.cmd -ErrorAction Stop).Source } catch {}
if (-not $npmCmd) { $npmCmd = (Get-Command npm).Source }
$preview = Start-Process -FilePath $npmCmd -ArgumentList "run","preview" -WorkingDirectory $web -PassThru -WindowStyle Hidden
Pop-Location

Start-Sleep -Seconds 3
try {
    Invoke-WebRequest -Uri $env:MANUAL_WEB_URL -UseBasicParsing -TimeoutSec 10 | Out-Null
} catch {
    Stop-Process -Id $preview.Id -Force -ErrorAction SilentlyContinue
    Write-Host "Web preview 未启动在 $env:MANUAL_WEB_URL" -ForegroundColor Red
    exit 1
}

Push-Location $shot
if (-not (Test-Path "node_modules/playwright")) {
    npm install
    npx playwright install chromium
}
npm run capture
$code = $LASTEXITCODE
Pop-Location

Stop-Process -Id $preview.Id -Force -ErrorAction SilentlyContinue
exit $code
