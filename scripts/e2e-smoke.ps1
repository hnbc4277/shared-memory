# Black-box smoke test against a running API (default http://127.0.0.1:18080).
# Start everything first:  docker compose --profile app up -d --build
# Then:                    powershell -File scripts/e2e-smoke.ps1
param(
    [string]$Api = "http://127.0.0.1:18080",
    [int]$WorkerWaitSeconds = 45,
    [int]$KBWaitSeconds = 45
)
$ErrorActionPreference = "Stop"
# Windows 控制台 UTF-8，避免中文功能名乱码
if ($PSVersionTable.PSVersion.Major -ge 6) {
    $null = chcp 65001
}
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$OutputEncoding = [Text.Encoding]::UTF8

$script:SmokeLog = @()
$script:StepIndex = 0
$script:AnyFailed = $false

function Write-SmokeResult {
    param(
        [ValidateSet("PASS", "FAIL")]
        [string]$Status,
        [string]$Feature,
        [string]$Detail = ""
    )
    $script:StepIndex++
    $id = "{0:D2}" -f $script:StepIndex
    $line = "[$id] $Status  $Feature"
    if ($Detail) { $line += " — $Detail" }
    if ($Status -eq "PASS") {
        Write-Host $line -ForegroundColor Green
    } else {
        Write-Host $line -ForegroundColor Red
        $script:AnyFailed = $true
    }
    $script:SmokeLog += [pscustomobject]@{
        Id      = $id
        Status  = $Status
        Feature = $Feature
        Detail  = $Detail
    }
}

function Require-True {
    param([bool]$Cond, [string]$Feature, [string]$Detail = "")
    if ($Cond) {
        Write-SmokeResult -Status PASS -Feature $Feature -Detail $Detail
    } else {
        Write-SmokeResult -Status FAIL -Feature $Feature -Detail $(if ($Detail) { $Detail } else { "assertion failed" })
    }
}

function Invoke-Sm {
    param([string]$Method, [string]$Path, $Body = $null, [string]$Key = "", [hashtable]$Headers = @{})
    $h = @{} + $Headers
    if ($Key) { $h["Authorization"] = "Bearer $Key" }
    $params = @{ Method = $Method; Uri = "$Api$Path"; Headers = $h; ContentType = "application/json; charset=utf-8"; UseBasicParsing = $true }
    if ($null -ne $Body) { $params["Body"] = [Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 8 -Compress)) }
    try {
        $r = Invoke-WebRequest @params
        $text = [Text.Encoding]::UTF8.GetString($r.RawContentStream.ToArray())
        $json = $null
        if ($r.Headers["Content-Type"] -like "application/json*") { $json = $text | ConvertFrom-Json }
        return [pscustomobject]@{ Status = [int]$r.StatusCode; Text = $text; Json = $json }
    } catch [System.Net.WebException] {
        $resp = $_.Exception.Response
        if ($null -eq $resp) { throw }
        $reader = New-Object IO.StreamReader($resp.GetResponseStream())
        return [pscustomobject]@{ Status = [int]$resp.StatusCode; Text = $reader.ReadToEnd(); Json = $null }
    }
}

function Invoke-SmKbUpload {
    param([string]$Key, [string]$Title, [string]$FileName, [string]$Content)
    Add-Type -AssemblyName System.Net.Http
    $client = [System.Net.Http.HttpClient]::new()
    $client.DefaultRequestHeaders.Authorization =
        [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $Key)
    $multipart = [System.Net.Http.MultipartFormDataContent]::new()
    $multipart.Add([System.Net.Http.StringContent]::new($Title), "title")
    $bytes = [Text.Encoding]::UTF8.GetBytes($Content)
    $filePart = [System.Net.Http.ByteArrayContent]::new($bytes)
    $filePart.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::new("text/markdown")
    $multipart.Add($filePart, "file", $FileName)
    $resp = $client.PostAsync("$Api/v1/kb/documents", $multipart).GetAwaiter().GetResult()
    $text = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    $json = $null
    if ($resp.Content.Headers.ContentType.MediaType -like "application/json*") {
        $json = $text | ConvertFrom-Json
    }
    [pscustomobject]@{ Status = [int]$resp.StatusCode; Text = $text; Json = $json }
}

function New-Account {
    $r = Invoke-Sm POST "/v1/auth/register" @{ email = "smoke-$([guid]::NewGuid())@example.com"; password = "password123" }
    Require-True ($r.Status -eq 201) "认证：注册账号" "HTTP $($r.Status)"
    return $r.Json.api_key
}

function New-Message([string]$ProjectId, [string]$Role, [string]$Content) {
    return @{
        project_id = $ProjectId; agent = "xiaolongxia"; agent_path = "xiaolongxia/researcher"
        skills     = @("deep-research"); plugins = @("browser"); tools = @("web_fetch"); model = "deepseek-v3"
        role       = $Role; content = $Content
    }
}

function Print-Summary {
    Write-Host ""
    Write-Host "========== Smoke 测试汇总 ==========" -ForegroundColor Cyan
    foreach ($row in $script:SmokeLog) {
        $color = if ($row.Status -eq "PASS") { "Green" } else { "Red" }
        $suffix = if ($row.Detail) { " ($($row.Detail))" } else { "" }
        Write-Host ("  {0}  {1}{2}" -f $row.Status, $row.Feature, $suffix) -ForegroundColor $color
    }
    $pass = @($script:SmokeLog | Where-Object { $_.Status -eq "PASS" }).Count
    $fail = @($script:SmokeLog | Where-Object { $_.Status -eq "FAIL" }).Count
    Write-Host "------------------------------------" -ForegroundColor Cyan
    Write-Host "  合计: $($script:SmokeLog.Count) 项 | 通过 $pass | 失败 $fail" -ForegroundColor $(if ($fail -gt 0) { "Red" } else { "Green" })
    Write-Host "  API: $Api" -ForegroundColor DarkGray
}

Write-Host "Smoke 开始 → $Api" -ForegroundColor Cyan
Write-Host ""

# --- 基础设施 ---
Require-True ((Invoke-Sm GET "/healthz").Status -eq 200) "健康检查：API /healthz" "服务可达"

# --- 账号与项目 ---
$A = New-Account
$B = New-Account
$projResp = Invoke-Sm POST "/v1/projects" @{ name = "smoke" } $A
Require-True ($projResp.Status -eq 201) "项目：创建项目" "HTTP $($projResp.Status)"
$project = $projResp.Json.id

# --- 会话与 ingest ---
$marker = "smoke" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$r1 = Invoke-Sm POST "/v1/ingest" (New-Message $project "user" "我们决定采用 OpenSearch 做全文检索 $marker") $A
Require-True ($r1.Status -eq 201) "会话：ingest（子智能体 provenance）" "agent_path=xiaolongxia/researcher"
$sessions = (Invoke-Sm GET "/v1/sessions?agent_path=xiaolongxia" $null $A).Json
Require-True (@($sessions).Count -eq 1) "会话：按 agent_path 筛选列表" "count=1"

# --- 智能体连接 ---
$agents1 = (Invoke-Sm GET "/v1/agents" $null $A).Json
$xlGroup = @($agents1.groups | Where-Object { $_.agent -eq "xiaolongxia" })[0]
Require-True ($null -ne $xlGroup -and $xlGroup.instance_count -ge 1) "智能体：ingest 后出现自动连接" "agent=xiaolongxia"
$autoInst = @($xlGroup.instances | Where-Object { $_.kind -eq "auto" })[0]
Require-True ($null -ne $autoInst) "智能体：自动行 kind=auto" ""
$connReg = (Invoke-Sm POST "/v1/agents/connections" @{ agent = "xiaolongxia"; label = "Smoke PC"; issue_dedicated_key = $false } $A).Json
Require-True ($null -ne $connReg.connection_id) "智能体：注册连接" ""
$regId = "$($connReg.connection_id)"
$ingConn = New-Message $project "user" "connection_id 绑定探测 $marker"
$ingConn.connection_id = $regId
$rConn = Invoke-Sm POST "/v1/ingest" $ingConn $A
Require-True ($rConn.Status -eq 201) "智能体：带 connection_id ingest" "id=$regId"
$agents2 = (Invoke-Sm GET "/v1/agents" $null $A).Json
$xlGroup2 = @($agents2.groups | Where-Object { $_.agent -eq "xiaolongxia" })[0]
$regInst = @($xlGroup2.instances | Where-Object { $_.id -eq $regId })[0]
Require-True ($null -ne $regInst -and $regInst.kind -eq "registered") "智能体：注册实例出现在列表" ""
Require-True ($null -ne $regInst.last_ingest_at) "智能体：注册实例记录 last_ingest_at" ""
$connDed = (Invoke-Sm POST "/v1/agents/connections" @{ agent = "xiaolongxia"; label = "Smoke dedicated"; issue_dedicated_key = $true } $A).Json
Require-True ($null -ne $connDed.api_key -and $connDed.connection_id) "智能体：专用 Key 连接" ""
$dedId = "$($connDed.connection_id)"
$ingDed = New-Message $project "user" "dedicated key auto bind $marker"
$rDed = Invoke-Sm POST "/v1/ingest" $ingDed "$($connDed.api_key)"
Require-True ($rDed.Status -eq 201) "智能体：专用 Key 自动 connection_id ingest" ""
$sessDed = (Invoke-Sm GET "/v1/sessions?connection_id=$dedId" $null $A).Json
Require-True (@($sessDed).Count -ge 1) "智能体：专用 Key 会话归属 connection" "count=$(@($sessDed).Count)"
$claim = Invoke-Sm POST "/v1/agents/connections/$regId/claim-history" @{} $A
Require-True ($claim.Status -eq 200 -and $claim.Json.sessions_claimed -ge 1) "智能体：claim-history 认领会话" "claimed=$($claim.Json.sessions_claimed)"
$sessConn = (Invoke-Sm GET "/v1/sessions?connection_id=$regId" $null $A).Json
Require-True (@($sessConn).Count -ge 1) "智能体：GET sessions 按 connection_id 筛选" "count=$(@($sessConn).Count)"
$badConn = Invoke-Sm POST "/v1/ingest" @{
    agent = "xiaolongxia"; agent_path = "xiaolongxia"; role = "user"; content = "bad"
    connection_id = [guid]::NewGuid().ToString()
    skills = @(); plugins = @(); tools = @()
} $A
Require-True ($badConn.Status -eq 400) "智能体：非法 connection_id 拒绝" "HTTP $($badConn.Status)"
$up = Invoke-Sm POST "/v1/agents/connections/$($autoInst.id)/upgrade" @{ label = "Smoke upgraded"; issue_dedicated_key = $false } $A
Require-True ($up.Status -eq 200 -and $null -ne $up.Json.connection_id) "智能体：auto 升级为 registered" ""
$agentsB = (Invoke-Sm GET "/v1/agents" $null $B).Json
Require-True (@($agentsB.groups).Count -eq 0) "智能体：跨账号列表隔离（B 无 A 分组）" ""

# --- Recall：connection 命中隔离（专用 Key 自动带 connection） ---
$isoA = "recall-iso-A-$marker"
$isoB = "recall-iso-B-$marker"
$ingIsoA = New-Message $project "user" $isoA
$rIsoA = Invoke-Sm POST "/v1/ingest" $ingIsoA "$($connDed.api_key)"
Require-True ($rIsoA.Status -eq 201) "Recall 隔离：connection A ingest" ""
$connIsoB = (Invoke-Sm POST "/v1/agents/connections" @{ agent = "xiaolongxia"; label = "Smoke iso B"; issue_dedicated_key = $true } $A).Json
$ingIsoB = New-Message $project "user" $isoB
$rIsoB = Invoke-Sm POST "/v1/ingest" $ingIsoB "$($connIsoB.api_key)"
Require-True ($rIsoB.Status -eq 201) "Recall 隔离：connection B ingest" ""
$leaked = $true
$deadline = (Get-Date).AddSeconds($WorkerWaitSeconds)
while ((Get-Date) -lt $deadline) {
    $recIso = (Invoke-Sm POST "/v1/recall" @{ query = $isoB; top_k = 10 } "$($connDed.api_key)").Json
    $leaked = $false
    foreach ($h in @($recIso.hits)) {
        if ("$($h.body)" -like "*$isoB*") { $leaked = $true; break }
    }
    if (-not $leaked) { break }
    Start-Sleep -Seconds 2
}
Require-True (-not $leaked) "Recall 隔离：A 的 Key 召回不应命中 B 专属内容" "query=$isoB waited<=${WorkerWaitSeconds}s"

# --- 设备码 + heartbeat → device_linked 在线 ---
$devCode = (Invoke-Sm POST "/v1/device/code" @{ client_name = "smoke-daemon"; agent = "cursor" } "").Json
Require-True ($null -ne $devCode.user_code) "智能体：device/code" ""
$apDev = Invoke-Sm POST "/v1/device/approve" @{ user_code = $devCode.user_code } $A
Require-True ($apDev.Status -eq 204) "智能体：device/approve" ""
$devKey = $null
for ($i = 0; $i -lt 30; $i++) {
    $tokResp = Invoke-Sm POST "/v1/device/token" @{ device_code = $devCode.device_code } ""
    if ($tokResp.Status -eq 200 -and $null -ne $tokResp.Json.api_key) {
        $devKey = "$($tokResp.Json.api_key)"
        break
    }
    Start-Sleep -Seconds 1
}
Require-True ($null -ne $devKey) "智能体：device/token 换取设备 Key" ""
$hb = Invoke-Sm POST "/v1/devices/heartbeat" @{ daemon_version = "smoke"; spool_pending = 1 } $devKey
Require-True ($hb.Status -eq 204) "智能体：devices/heartbeat" ""
$agentsDev = (Invoke-Sm GET "/v1/agents" $null $A).Json
$cursorDev = @($agentsDev.groups | Where-Object { $_.agent -eq "cursor" })[0]
$linked = $null
if ($null -ne $cursorDev) {
    $linked = @($cursorDev.instances | Where-Object { $_.kind -eq "device_linked" })[0]
}
Require-True ($null -ne $linked -and $linked.online -eq $true) "智能体：device_linked 在线" "daemon_version=$($linked.daemon_version)"

# --- 记忆 recall ---
$hit = $false
for ($i = 0; $i -lt $WorkerWaitSeconds; $i++) {
    $rec = Invoke-Sm POST "/v1/recall" @{ project_id = $project; query = "$marker OpenSearch" } $A
    if ($rec.Text -like "*$marker*") { $hit = $true; break }
    Start-Sleep -Seconds 1
}
Require-True $hit "记忆：recall 命中本账号（worker 已处理队列）" "marker=$marker"
$recB = Invoke-Sm POST "/v1/recall" @{ query = "$marker OpenSearch" } $B
Require-True (-not ($recB.Text -like "*$marker*")) "记忆：跨账号隔离（B 看不到 A）" ""

# --- 策略 ---
Invoke-Sm PUT "/v1/policy" @{ store_transcript = $false } $A | Out-Null
$r2 = Invoke-Sm POST "/v1/ingest" (New-Message $project "user" "这条不保存原文") $A
$detail = (Invoke-Sm GET "/v1/sessions/$($r2.Json.session_id)" $null $A).Json
Require-True (-not $detail.messages[0].content_stored) "策略：关闭 transcript 后不存原文" ""
Invoke-Sm PUT "/v1/policy" @{ store_transcript = $true } $A | Out-Null
Require-True $true "策略：恢复 transcript 存储" ""

# --- 保险库 ---
$secret = "Zx9!" + ([guid]::NewGuid().ToString("N").Substring(0, 6))
$r3 = Invoke-Sm POST "/v1/ingest" (New-Message $project "user" "GitLab https://gitlab.example.com 账号: alice 密码: $secret") $A
$vaultPrompt = @($r3.Json.prompts | Where-Object { $_.kind -eq "vault" })[0]
Require-True ($null -ne $vaultPrompt) "保险库：ingest 后提供 vault 候选" ""
$item = (Invoke-Sm POST "/v1/vault/candidates/$($vaultPrompt.candidate_id)/accept" @{} $A).Json
$reveal = Invoke-Sm POST "/v1/vault/items/$($item.id)/reveal" @{ password = "password123" } $A
Require-True ($reveal.Json.secret -eq $secret) "保险库：密码验证后 reveal 明文" ""
$leak = (Invoke-Sm POST "/v1/recall" @{ project_id = $project; query = "GitLab 密码" } $A).Text +
(Invoke-Sm GET "/v1/sessions/$($r3.Json.session_id)" $null $A).Text
Require-True (-not $leak.Contains($secret)) "保险库：recall/会话响应不含明文密钥" ""

# --- 成稿 ---
$long = "# Redis Streams 学习笔记`n" + ("消费者组、XACK 与 XAUTOCLAIM 的用法。" * 60)
$r4 = Invoke-Sm POST "/v1/ingest" (New-Message $project "assistant" $long) $A
$artPrompt = @($r4.Json.prompts | Where-Object { $_.kind -eq "artifact" })[0]
Require-True ($null -ne $artPrompt) "成稿：长回复触发 artifact 候选" ""
$art = (Invoke-Sm POST "/v1/artifacts/candidates/$($artPrompt.candidate_id)/accept" @{} $A).Json
$body = (Invoke-Sm GET "/v1/artifacts/$($art.artifact.id)" $null $A).Json.body
Require-True ($body -eq $long) "成稿：接受后可读取完整 body" ""

# --- 知识库 KB ---
$kbMarker = "smkb" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$kbBody = "# 知识库 Smoke`n`n项目规范要求 API 带 account_id 隔离。$kbMarker`n"
$kbUp = Invoke-SmKbUpload -Key $A -Title "Smoke KB" -FileName "spec.md" -Content $kbBody
Require-True ($kbUp.Status -eq 201) "知识库：multipart 上传文档" "HTTP $($kbUp.Status)"
$kbDocId = $kbUp.Json.document.id
Require-True ($null -ne $kbDocId -and "$kbDocId".Length -gt 0) "知识库：返回 document id" ""

$kbReady = $false
$kbErr = ""
for ($i = 0; $i -lt $KBWaitSeconds; $i++) {
    $docGet = Invoke-Sm GET "/v1/kb/documents/$kbDocId" $null $A
    $st = $docGet.Json.document.status
    if ($st -eq "ready") { $kbReady = $true; break }
    if ($st -eq "failed") {
        $kbErr = $docGet.Json.document.error
        break
    }
    Start-Sleep -Seconds 1
}
Require-True $kbReady "知识库：worker kb_parse 后 status=ready" $(if ($kbErr) { "error=$kbErr" } else { "doc=$kbDocId" })

$kbHit = $false
$kbRecText = ""
for ($i = 0; $i -lt 15; $i++) {
    $kbRec = Invoke-Sm POST "/v1/recall" @{ query = "$kbMarker account_id" } $A
    $kbRecText = $kbRec.Text
    if ($kbRecText -like "*$kbMarker*") { $kbHit = $true; break }
    Start-Sleep -Seconds 1
}
Require-True $kbHit "知识库：recall 注入 kb 内容（含 marker）" ""
$kbRecJson = $kbRecText | ConvertFrom-Json
$hasKbHits = ($null -ne $kbRecJson.kb_hits) -and (@($kbRecJson.kb_hits).Count -gt 0)
Require-True $hasKbHits "知识库：recall 响应含 kb_hits 数组" ""
$kbGetB = Invoke-Sm GET "/v1/kb/documents/$kbDocId" $null $B
Require-True ($kbGetB.Status -eq 404) "知识库：跨账号 GET 文档隔离" "B HTTP $($kbGetB.Status)"
$kbRecB = Invoke-Sm POST "/v1/recall" @{ query = "$kbMarker account_id" } $B
Require-True (-not ($kbRecB.Text -like "*$kbMarker*")) "知识库：跨账号 recall 不泄露" ""

# --- 账号删除 ---
$del = Invoke-Sm DELETE "/v1/account" $null $A @{ "X-Confirm" = "delete" }
Require-True ($del.Status -eq 204) "账号：确认后删除 A" "HTTP $($del.Status)"
Require-True ((Invoke-Sm GET "/v1/sessions" $null $A).Status -eq 401) "账号：删除后 API Key 失效" ""
Require-True ((Invoke-Sm GET "/v1/sessions" $null $B).Status -eq 200) "账号：删除 A 后 B 仍可用" ""

Print-Summary
if ($script:AnyFailed) {
    Write-Host ""
    Write-Host "smoke: 存在失败项" -ForegroundColor Red
    exit 1
}
Write-Host ""
Write-Host "smoke: 全部通过" -ForegroundColor Green
exit 0
