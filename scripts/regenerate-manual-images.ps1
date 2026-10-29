# Regenerate docs/公开/manual PNG wireframes (UTF-8 SVG, Chinese labels)
$dir = Join-Path $PSScriptRoot "..\docs\manual\images" | Resolve-Path
$utf8 = New-Object System.Text.UTF8Encoding $false
$ff = "Microsoft YaHei, SimHei, sans-serif"

function Write-Svg($name, $svg) {
  [IO.File]::WriteAllText((Join-Path $dir "$name.svg"), $svg, $utf8)
}

Write-Svg "01-console-shell" @"
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 920 520">
  <rect width="920" height="520" fill="#f4f6f8"/>
  <rect width="200" height="520" fill="#1a2332"/>
  <text x="24" y="36" fill="#fff" font-family="$ff" font-size="15" font-weight="600">共享记忆</text>
  <text x="24" y="115" fill="#fff" font-family="$ff">历史</text>
  <text x="24" y="143" fill="#c5d0de" font-family="$ff">记忆</text>
  <text x="24" y="171" fill="#c5d0de" font-family="$ff">智能体</text>
  <text x="24" y="236" fill="#c5d0de" font-family="$ff">凭证库</text>
  <rect x="200" y="0" width="720" height="56" fill="#fff" stroke="#dde3ea"/>
  <text x="220" y="34" fill="#333" font-family="$ff" font-size="18" font-weight="600">历史</text>
  <rect x="620" y="14" width="88" height="28" rx="14" fill="#fff3e0" stroke="#ffb74d"/>
  <text x="628" y="33" fill="#e65100" font-family="$ff" font-size="11">待确认 2</text>
  <rect x="718" y="16" width="120" height="26" rx="4" fill="#f0f3f6" stroke="#ccc"/>
  <text x="728" y="33" fill="#555" font-family="$ff" font-size="11">全部/全局</text>
  <rect x="212" y="64" width="696" height="44" rx="8" fill="#e3f2fd" stroke="#90caf9"/>
  <text x="228" y="90" fill="#1565c0" font-family="$ff" font-size="12">2/5 连接在线 · 点击查看智能体</text>
  <rect x="212" y="120" width="696" height="380" rx="8" fill="#fff" stroke="#dde3ea"/>
  <text x="240" y="160" fill="#888" font-family="$ff">主内容区</text>
</svg>
"@

Write-Svg "02-login" @"
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 500 320">
  <rect fill="#eef1f5" width="500" height="320"/>
  <rect x="40" y="40" width="420" height="240" rx="12" fill="#fff" stroke="#ccc"/>
  <text x="60" y="75" font-family="$ff" font-size="14">欢迎回来</text>
  <rect x="60" y="95" width="100" height="28" rx="6" fill="#1976d2"/>
  <text x="72" y="113" fill="#fff" font-family="$ff" font-size="11">API 密钥</text>
  <text x="60" y="155" font-family="$ff" font-size="11">粘贴 sm_ 开头密钥</text>
  <rect x="60" y="190" width="200" height="36" fill="#1976d2"/>
  <text x="160" y="213" fill="#fff" text-anchor="middle" font-family="$ff">进入控制台</text>
</svg>
"@

Write-Svg "02b-login-email" @"
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 500 320">
  <rect fill="#eef1f5" width="500" height="320"/>
  <rect x="40" y="40" width="420" height="240" rx="12" fill="#fff" stroke="#ccc"/>
  <text x="60" y="75" font-family="$ff" font-size="14">邮箱登录</text>
  <text x="60" y="110" font-family="$ff" font-size="11">邮箱</text>
  <text x="60" y="150" font-family="$ff" font-size="11">密码</text>
  <rect x="60" y="180" width="200" height="36" fill="#1976d2"/>
  <text x="160" y="203" fill="#fff" text-anchor="middle" font-family="$ff">登录</text>
</svg>
"@

Write-Svg "02c-register" @"
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 500 360">
  <rect fill="#eef1f5" width="500" height="360"/>
  <rect x="40" y="40" width="420" height="280" rx="12" fill="#fff" stroke="#ccc"/>
  <text x="60" y="75" font-family="$ff" font-size="14">注册账号</text>
  <rect x="60" y="200" width="380" height="80" fill="#f5f5f5" stroke="#999"/>
  <text x="70" y="230" font-family="$ff" font-size="11">API 密钥（仅显示一次）</text>
  <text x="70" y="260" font-family="$ff" font-size="11">复制 · 已保存，进入</text>
</svg>
"@

$pages = @{
  "03-history-split" = "历史 · 会话列表与详情"
  "04-agents" = "智能体 · 连接与在线"
  "05-memory" = "记忆 · 召回测试与列表"
  "06-vault" = "凭证库 · 待确认与已保存"
  "07-artifacts" = "成稿库 · 候选与导出"
  "08-kb" = "知识库 · 上传与解析"
  "09-projects" = "项目 · 创建与 project_id"
  "10-keys" = "密钥与回执"
  "11-device" = "授权设备 · 用户码"
  "12-settings" = "设置 · 策略与隔离区"
}
foreach ($kv in $pages.GetEnumerator()) {
  $id = $kv.Key
  $title = $kv.Value
  Write-Svg $id @"
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 280">
  <rect fill="#f4f6f8" width="800" height="280"/>
  <text x="20" y="40" font-family="$ff" font-size="16" font-weight="600">$title</text>
  <rect x="20" y="60" width="760" height="200" rx="8" fill="#fff" stroke="#dde3ea"/>
  <text x="40" y="120" font-family="$ff" font-size="13" fill="#555">界面示意图 · 详见浏览器控制台</text>
</svg>
"@
}

Get-ChildItem (Join-Path $dir "*.svg") | ForEach-Object {
  $png = $_.FullName -replace '\.svg$','.png'
  npx --yes @resvg/resvg-js-cli $_.FullName $png 2>&1 | Out-Null
  Write-Host "OK $($_.Name) -> $(Split-Path $png -Leaf)"
}

Copy-Item (Join-Path $dir "*.png") (Join-Path $PSScriptRoot "..\web\public\docs\manual\images") -Force
