#Requires -Version 5.1
param(
  [string]$ApiUrl,
  [string]$ApiKey,
  [string]$ConnectionId,
  [string]$CursorConfigDir,
  [string]$InstallDir,
  [string]$DownloadBase,
  [string]$RepoRoot,
  [switch]$SkipDownload
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ASCII-only notes after param(). Server replaces __SM_*__ once; do not eq-compare to those tokens later.
if ([string]::IsNullOrWhiteSpace($ApiUrl)) { $ApiUrl = '__SM_API_URL__' }
if ([string]::IsNullOrWhiteSpace($ApiKey)) { $ApiKey = '__SM_API_KEY__' }
if ([string]::IsNullOrWhiteSpace($ConnectionId)) { $ConnectionId = '__SM_CONNECTION_ID__' }
if ([string]::IsNullOrWhiteSpace($DownloadBase)) { $DownloadBase = '__SM_DOWNLOAD_BASE__' }

if ($ApiUrl -match '__SM_' -or [string]::IsNullOrWhiteSpace($ApiUrl)) {
  $ApiUrl = 'http://127.0.0.1:18080'
}
if ($DownloadBase -match '__SM_' -or [string]::IsNullOrWhiteSpace($DownloadBase)) {
  $DownloadBase = 'http://127.0.0.1:15173'
}
# Real dedicated keys look like sm_...; placeholders / empty do not.
if ($ApiKey -notlike 'sm_*') { $ApiKey = '' }
if ($ConnectionId -notmatch '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-') { $ConnectionId = '' }
$CredentialsBaked = (-not [string]::IsNullOrWhiteSpace($ApiKey)) -and (-not [string]::IsNullOrWhiteSpace($ConnectionId))

function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Write-Ok([string]$msg) { Write-Host "    $msg" -ForegroundColor Green }
function Write-WarnLine([string]$msg) { Write-Host "    $msg" -ForegroundColor Yellow }

function Get-RepoRoot {
  if ($RepoRoot -and (Test-Path $RepoRoot)) { return (Resolve-Path $RepoRoot).Path }
  $here = $PSScriptRoot
  if (-not $here) { $here = Split-Path -Parent $MyInvocation.MyCommand.Path }
  return (Resolve-Path (Join-Path $here "..")).Path
}

function Resolve-CursorExeFromPath([string]$path) {
  if (-not $path -or -not (Test-Path -LiteralPath $path)) { return $null }
  if ($path -match '(?i)Cursor\.exe$') { return $path }
  # PATH 上常见 resources\app\bin\cursor.cmd → 上溯找 Cursor.exe
  $dir = Split-Path -Parent $path
  for ($i = 0; $i -lt 6; $i++) {
    $exe = Join-Path $dir "Cursor.exe"
    if (Test-Path -LiteralPath $exe) { return $exe }
    $parent = Split-Path -Parent $dir
    if (-not $parent -or $parent -eq $dir) { break }
    $dir = $parent
  }
  return $path
}

function Find-CursorExe {
  $candidates = @(
    (Join-Path $env:LOCALAPPDATA "Programs\cursor\Cursor.exe"),
    (Join-Path $env:LOCALAPPDATA "Programs\Cursor\Cursor.exe"),
    (Join-Path $env:LOCALAPPDATA "cursor\Cursor.exe"),
    (Join-Path ${env:ProgramFiles} "Cursor\Cursor.exe"),
    "G:\Program Files\cursor\Cursor.exe",
    "D:\Program Files\cursor\Cursor.exe",
    "E:\Program Files\cursor\Cursor.exe"
  )
  if (${env:ProgramFiles(x86)}) {
    $candidates += (Join-Path ${env:ProgramFiles(x86)} "Cursor\Cursor.exe")
  }
  foreach ($p in $candidates) {
    if ($p -and (Test-Path -LiteralPath $p)) { return $p }
  }
  $cmd = Get-Command cursor -ErrorAction SilentlyContinue
  if ($cmd -and $cmd.Source) {
    $resolved = Resolve-CursorExeFromPath $cmd.Source
    if ($resolved) { return $resolved }
  }
  $uninstallRoots = @(
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*",
    "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*",
    "HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*"
  )
  foreach ($root in $uninstallRoots) {
    foreach ($item in (Get-ItemProperty $root -ErrorAction SilentlyContinue)) {
      $name = [string]$item.DisplayName
      if ($name -notmatch '(?i)cursor') { continue }
      $loc = [string]$item.InstallLocation
      if (-not $loc) { continue }
      $exe = Join-Path $loc "Cursor.exe"
      if (Test-Path -LiteralPath $exe) { return $exe }
    }
  }
  return $null
}

function Resolve-CursorConfigDir {
  param([string]$Override, [string]$CursorExe)

  if ($Override) {
    if (-not (Test-Path -LiteralPath $Override)) {
      New-Item -ItemType Directory -Path $Override -Force | Out-Null
    }
    return (Resolve-Path -LiteralPath $Override).Path
  }
  if ($env:CURSOR_CONFIG_DIR) {
    $p = $env:CURSOR_CONFIG_DIR
    if (-not (Test-Path -LiteralPath $p)) {
      New-Item -ItemType Directory -Path $p -Force | Out-Null
    }
    return (Resolve-Path -LiteralPath $p).Path
  }

  $homeCursor = Join-Path $env:USERPROFILE ".cursor"
  $candidates = New-Object System.Collections.Generic.List[string]
  [void]$candidates.Add($homeCursor)

  # 便携/自定义：Cursor.exe 旁的 data\.cursor 或 .cursor
  if ($CursorExe) {
    $exeDir = Split-Path -Parent $CursorExe
    foreach ($rel in @(".cursor", "data\.cursor", "..\..\.cursor")) {
      $c = [System.IO.Path]::GetFullPath((Join-Path $exeDir $rel))
      if (-not $candidates.Contains($c)) { [void]$candidates.Add($c) }
    }
  }

  # 优先：已有 hooks.json / mcp.json 的目录
  foreach ($c in $candidates) {
    if ((Test-Path -LiteralPath (Join-Path $c "hooks.json")) -or
        (Test-Path -LiteralPath (Join-Path $c "mcp.json"))) {
      return $c
    }
  }
  # 其次：已存在的 .cursor 目录
  foreach ($c in $candidates) {
    if (Test-Path -LiteralPath $c) { return $c }
  }

  New-Item -ItemType Directory -Path $homeCursor -Force | Out-Null
  return $homeCursor
}

function Read-JsonFile([string]$path) {
  if (-not (Test-Path -LiteralPath $path)) { return $null }
  $raw = Get-Content -LiteralPath $path -Raw -Encoding UTF8
  if ([string]::IsNullOrWhiteSpace($raw)) { return $null }
  return $raw | ConvertFrom-Json
}

function Write-JsonFile([string]$path, $obj) {
  $dir = Split-Path -Parent $path
  if (-not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }
  $json = $obj | ConvertTo-Json -Depth 20
  # UTF-8 无 BOM，避免部分工具解析失败
  $utf8 = New-Object System.Text.UTF8Encoding $false
  [System.IO.File]::WriteAllText($path, $json + "`n", $utf8)
}

function Get-Prop($obj, [string]$name) {
  if ($null -eq $obj) { return $null }
  $p = $obj.PSObject.Properties[$name]
  if ($null -eq $p) { return $null }
  return $p.Value
}

function Ensure-NoteProperty($obj, [string]$name, $value) {
  if ($obj.PSObject.Properties[$name]) {
    $obj.$name = $value
  } else {
    $obj | Add-Member -NotePropertyName $name -NotePropertyValue $value
  }
}

function ConvertTo-ArrayList($value) {
  $list = New-Object System.Collections.ArrayList
  if ($null -eq $value) { return $list }
  # PS 5.1 的 ConvertFrom-Json 会把单元素数组收成单个对象
  if ($value -is [string]) {
    [void]$list.Add($value)
    return $list
  }
  if ($value -is [System.Array]) {
    foreach ($item in $value) { [void]$list.Add($item) }
    return $list
  }
  if ($value -is [System.Collections.IList]) {
    foreach ($item in $value) { [void]$list.Add($item) }
    return $list
  }
  [void]$list.Add($value)
  return $list
}

function Upsert-NamedHook($list, [string]$name, [string]$command) {
  $arr = New-Object System.Collections.ArrayList
  foreach ($item in @($list)) {
    if ($null -ne $item) { [void]$arr.Add($item) }
  }
  $found = $false
  for ($i = 0; $i -lt $arr.Count; $i++) {
    $item = $arr[$i]
    $n = [string](Get-Prop $item "name")
    if ($n -eq $name) {
      Ensure-NoteProperty $item "name" $name
      Ensure-NoteProperty $item "command" $command
      $arr[$i] = $item
      $found = $true
      break
    }
  }
  if (-not $found) {
    [void]$arr.Add([pscustomobject]@{ name = $name; command = $command })
  }
  return ,@($arr.ToArray())
}

function Install-Binary {
  param(
    [string]$Name,
    [string]$DestPath,
    [string]$DownloadBaseUrl,
    [string]$AltDownloadBaseUrl,
    [string]$LocalFallback,
    [switch]$SkipDl
  )
  $destDir = Split-Path -Parent $DestPath
  if (-not (Test-Path -LiteralPath $destDir)) {
    New-Item -ItemType Directory -Path $destDir -Force | Out-Null
  }

  if ((Test-Path -LiteralPath $DestPath) -and $SkipDl) {
    Write-Ok "已存在，跳过下载: $DestPath"
    return
  }

  $bases = @()
  foreach ($b in @($DownloadBaseUrl, $AltDownloadBaseUrl)) {
    $b = [string]$b
    if ([string]::IsNullOrWhiteSpace($b)) { continue }
    $b = $b.TrimEnd("/")
    if ($bases -notcontains $b) { $bases += $b }
  }

  if (-not $SkipDl) {
    $tmp = "$DestPath.download"
    foreach ($base in $bases) {
      $url = "$base/downloads/$Name"
      try {
        Write-Host "    尝试下载 $url"
        Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing -TimeoutSec 60
        Move-Item -LiteralPath $tmp -Destination $DestPath -Force
        Write-Ok "已安装: $DestPath"
        return
      } catch {
        Write-WarnLine "下载失败: $($_.Exception.Message)"
        if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
      }
    }
  }

  if ($LocalFallback -and (Test-Path -LiteralPath $LocalFallback)) {
    Copy-Item -LiteralPath $LocalFallback -Destination $DestPath -Force
    Write-Ok "已从仓库复制: $LocalFallback -> $DestPath"
    return
  }

  if (Test-Path -LiteralPath $DestPath) {
    Write-Ok "沿用已有文件: $DestPath"
    return
  }

  throw "无法获取 $Name。请确认 Shared Memory API 已启动（默认 http://127.0.0.1:18080），或用 -DownloadBase 指定可访问的 API 地址。"
}

function Prompt-IfEmpty([string]$current, [string]$label, [switch]$Secret) {
  if (-not [string]::IsNullOrWhiteSpace($current)) { return $current.Trim() }
  if ($Secret) {
    $secure = Read-Host -Prompt $label -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
      return ([Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)).Trim()
    } finally {
      [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
  }
  return (Read-Host -Prompt $label).Trim()
}

# --- main ---
$root = Get-RepoRoot
# 安装目录：-InstallDir > 环境变量 SHARED_MEMORY_HOME > 当前用户主目录下的 .shared-memory
# 不写死任何用户名路径；换机器/换账号会自动落到对应用户目录。
if ($InstallDir) {
  $installDir = $InstallDir
} elseif ($env:SHARED_MEMORY_HOME) {
  $installDir = $env:SHARED_MEMORY_HOME
} else {
  $installDir = Join-Path $env:USERPROFILE ".shared-memory"
}
$installDir = [System.IO.Path]::GetFullPath($installDir)
$smhookDest = Join-Path $installDir "smhook.exe"
$mcpDest = Join-Path $installDir "shared-memory-mcp.exe"
$credPath = Join-Path $installDir "credentials.json"
$localSmhook = Join-Path $root "web\public\downloads\smhook.exe"
$localMcp = Join-Path $root "web\public\downloads\shared-memory-mcp.exe"

Write-Step "探测 Cursor"
$cursorExe = Find-CursorExe
if ($cursorExe) {
  Write-Ok "找到 Cursor 程序: $cursorExe"
} else {
  Write-WarnLine "未找到 Cursor.exe（仍会写入用户配置目录；若你用便携版请加 -CursorConfigDir）"
}
$configDir = Resolve-CursorConfigDir -Override $CursorConfigDir -CursorExe $cursorExe
Write-Ok "Cursor 配置目录: $configDir"

Write-Step "安装共享记忆程序"
Write-Ok "目标目录: $installDir  (可用 -InstallDir 或 SHARED_MEMORY_HOME 覆盖)"
Install-Binary -Name "smhook.exe" -DestPath $smhookDest -DownloadBaseUrl $DownloadBase.TrimEnd("/") `
  -AltDownloadBaseUrl $ApiUrl.TrimEnd("/") -LocalFallback $localSmhook -SkipDl:$SkipDownload
Install-Binary -Name "shared-memory-mcp.exe" -DestPath $mcpDest -DownloadBaseUrl $DownloadBase.TrimEnd("/") `
  -AltDownloadBaseUrl $ApiUrl.TrimEnd("/") -LocalFallback $localMcp -SkipDl:$SkipDownload

Write-Step "凭据"
$existing = Read-JsonFile $credPath
if ($existing) {
  if (-not $CredentialsBaked -and (-not $ApiUrl -or $ApiUrl -eq "http://127.0.0.1:18080")) {
    $eu = [string](Get-Prop $existing "api_url")
    if ($eu) { $ApiUrl = $eu }
  }
  if (-not $ApiKey) {
    $ek = [string](Get-Prop $existing "api_key")
    if ($ek) { $ApiKey = $ek; Write-Ok "复用已有 api_key" }
  }
  if (-not $ConnectionId) {
    $ec = [string](Get-Prop $existing "connection_id")
    if ($ec) { $ConnectionId = $ec; Write-Ok "复用已有 connection_id" }
  }
}

if ($CredentialsBaked -and -not [string]::IsNullOrWhiteSpace($ApiKey) -and -not [string]::IsNullOrWhiteSpace($ConnectionId)) {
  Write-Ok "使用脚本内置凭据（无需输入 API 密钥）"
  Write-Ok "API: $ApiUrl"
  Write-Ok "connection_id: $ConnectionId"
} else {
  $ApiUrl = Prompt-IfEmpty $ApiUrl "API 地址 (例如 http://127.0.0.1:18080)"
  $ApiKey = Prompt-IfEmpty $ApiKey "专用 API 密钥 (sm_...)" -Secret
  $ConnectionId = Prompt-IfEmpty $ConnectionId "connection_id (控制台智能体页)"
}

if ([string]::IsNullOrWhiteSpace($ApiKey)) { throw "缺少 API 密钥（请重新从控制台下载该连接的配置包）" }
if ([string]::IsNullOrWhiteSpace($ConnectionId)) { throw "缺少 connection_id（请重新从控制台下载该连接的配置包）" }

$credObj = [pscustomobject]@{
  api_url       = $ApiUrl.TrimEnd("/")
  api_key       = $ApiKey
  connection_id = $ConnectionId
}
Write-JsonFile $credPath $credObj
Write-Ok "已写入 $credPath"

Write-Step "合并 hooks.json（保留已有 sessionStart）"
$hooksPath = Join-Path $configDir "hooks.json"
$hookCmd = "cmd /c `"set SHARED_MEMORY_HOOK_SURFACE=cursor&& $smhookDest`""
$hooksRoot = Read-JsonFile $hooksPath
if (-not $hooksRoot) {
  $hooksRoot = [pscustomobject]@{ version = 1; hooks = [pscustomobject]@{} }
}
if (-not (Get-Prop $hooksRoot "version")) { Ensure-NoteProperty $hooksRoot "version" 1 }
$hooksMap = Get-Prop $hooksRoot "hooks"
if (-not $hooksMap) {
  $hooksMap = [pscustomobject]@{}
  Ensure-NoteProperty $hooksRoot "hooks" $hooksMap
}
foreach ($ev in @("beforeSubmitPrompt", "afterAgentResponse", "subagentStop")) {
  $list = ConvertTo-ArrayList (Get-Prop $hooksMap $ev)
  $merged = Upsert-NamedHook $list "shared-memory" $hookCmd
  Ensure-NoteProperty $hooksMap $ev @($merged)
}
# 备份
if (Test-Path -LiteralPath $hooksPath) {
  Copy-Item -LiteralPath $hooksPath -Destination ($hooksPath + ".bak") -Force
}
Write-JsonFile $hooksPath $hooksRoot
Write-Ok "已更新 $hooksPath"

Write-Step "合并 mcp.json（保留 officecli 等已有项）"
$mcpPath = Join-Path $configDir "mcp.json"
$mcpRoot = Read-JsonFile $mcpPath
if (-not $mcpRoot) {
  $mcpRoot = [pscustomobject]@{ mcpServers = [pscustomobject]@{} }
}
$servers = Get-Prop $mcpRoot "mcpServers"
if (-not $servers) {
  $servers = [pscustomobject]@{}
  Ensure-NoteProperty $mcpRoot "mcpServers" $servers
}
$smEntry = [pscustomobject]@{
  command = $mcpDest
  env     = [pscustomobject]@{
    SHARED_MEMORY_API_URL       = $ApiUrl.TrimEnd("/")
    SHARED_MEMORY_API_KEY       = $ApiKey
    SHARED_MEMORY_AGENT         = "cursor"
    SHARED_MEMORY_CONNECTION_ID = $ConnectionId
  }
}
Ensure-NoteProperty $servers "shared-memory" $smEntry
if (Test-Path -LiteralPath $mcpPath) {
  Copy-Item -LiteralPath $mcpPath -Destination ($mcpPath + ".bak") -Force
}
Write-JsonFile $mcpPath $mcpRoot
Write-Ok "已更新 $mcpPath"

Write-Step "写入召回规则"
$rulesDir = Join-Path $configDir "rules"
$rulePath = Join-Path $rulesDir "shared-memory.mdc"
$rule = @"
---
description: Shared Memory - recall at the start of non-trivial tasks
globs:
alwaysApply: true
---

# Shared Memory

每个非琐碎任务开始时：

1. 调用 memory_recall。query 用当前用户目标的一句话。
2. 把返回的 context 当作已知背景，不要让用户重复已经记住的偏好和决定。
3. 不要调用 memory_ingest。用户消息和助手回复已由钩子写入。
4. 不要在对话或工具参数里打印 API Key。
5. memory_recall 失败时继续任务，不要卡住。
"@
if (-not (Test-Path -LiteralPath $rulesDir)) {
  New-Item -ItemType Directory -Path $rulesDir -Force | Out-Null
}
$utf8 = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($rulePath, $rule.TrimStart() + "`n", $utf8)
Write-Ok "已写入 $rulePath"

Write-Host ""
Write-Host "配置完成。" -ForegroundColor Green
Write-Host "  Cursor 程序:   $(if ($cursorExe) { $cursorExe } else { '(未检测到)' })"
Write-Host "  配置目录:     $configDir"
Write-Host "  程序目录:     $installDir"
Write-Host "  connection:   $ConnectionId"
Write-Host ""
Write-Host "请完全退出 Cursor 后重新打开，发一句测试话，再到控制台「智能体」看最后活动。" -ForegroundColor Yellow
Write-Host "不要另开 shared-memory-daemon (127.0.0.1:18765)。" -ForegroundColor Yellow
