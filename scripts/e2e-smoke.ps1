# Black-box smoke test against a running API (default http://127.0.0.1:18080).
# Start everything first:  docker compose --profile app up -d --build
# Then:                    powershell -File scripts/e2e-smoke.ps1
param(
    [string]$Api = "http://127.0.0.1:18080",
    [int]$WorkerWaitSeconds = 20
)
$ErrorActionPreference = "Stop"

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

function Assert-True([bool]$Cond, [string]$Msg) {
    if (-not $Cond) { Write-Host "FAIL: $Msg" -ForegroundColor Red; exit 1 }
    Write-Host "ok   $Msg" -ForegroundColor Green
}

function New-Account {
    $r = Invoke-Sm POST "/v1/auth/register" @{ email = "smoke-$([guid]::NewGuid())@example.com"; password = "password123" }
    Assert-True ($r.Status -eq 201) "register account"
    return $r.Json.api_key
}

function New-Message([string]$ProjectId, [string]$Role, [string]$Content) {
    return @{ project_id = $ProjectId; agent = "xiaolongxia"; agent_path = "xiaolongxia/researcher"
        skills = @("deep-research"); plugins = @("browser"); tools = @("web_fetch"); model = "deepseek-v3"
        role = $Role; content = $Content }
}

Assert-True ((Invoke-Sm GET "/healthz").Status -eq 200) "api healthy at $Api"

# 2. two accounts
$A = New-Account
$B = New-Account
$project = (Invoke-Sm POST "/v1/projects" @{ name = "smoke" } $A).Json.id

# 3. sub-agent ingest with provenance arrays
$marker = "smoke" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$r1 = Invoke-Sm POST "/v1/ingest" (New-Message $project "user" "我们决定采用 OpenSearch 做全文检索 $marker") $A
Assert-True ($r1.Status -eq 201) "ingest with agent_path=xiaolongxia/researcher"
$sessions = (Invoke-Sm GET "/v1/sessions?agent_path=xiaolongxia" $null $A).Json
Assert-True (@($sessions).Count -eq 1) "session listed under agent_path filter"

# 4. recall: A hits (after the worker processes the job), B sees nothing
$hit = $false
for ($i = 0; $i -lt $WorkerWaitSeconds; $i++) {
    $rec = Invoke-Sm POST "/v1/recall" @{ project_id = $project; query = "$marker OpenSearch" } $A
    if ($rec.Text -like "*$marker*") { $hit = $true; break }
    Start-Sleep -Seconds 1
}
Assert-True $hit "A recall returns its memory (worker processed the job)"
$recB = Invoke-Sm POST "/v1/recall" @{ query = "$marker OpenSearch" } $B
Assert-True (-not ($recB.Text -like "*$marker*")) "B recall does not contain A's data"

# 5. transcript off -> content not stored
Invoke-Sm PUT "/v1/policy" @{ store_transcript = $false } $A | Out-Null
$r2 = Invoke-Sm POST "/v1/ingest" (New-Message $project "user" "这条不保存原文") $A
$detail = (Invoke-Sm GET "/v1/sessions/$($r2.Json.session_id)" $null $A).Json
Assert-True (-not $detail.messages[0].content_stored) "transcript off: message content not stored"
Invoke-Sm PUT "/v1/policy" @{ store_transcript = $true } $A | Out-Null

# 6. vault: consent -> reveal with password; secret never in recall/session output
$secret = "Zx9!" + ([guid]::NewGuid().ToString("N").Substring(0, 6))
$r3 = Invoke-Sm POST "/v1/ingest" (New-Message $project "user" "GitLab https://gitlab.example.com 账号: alice 密码: $secret") $A
$vaultPrompt = @($r3.Json.prompts | Where-Object { $_.kind -eq "vault" })[0]
Assert-True ($null -ne $vaultPrompt) "vault candidate offered"
$item = (Invoke-Sm POST "/v1/vault/candidates/$($vaultPrompt.candidate_id)/accept" @{} $A).Json
$reveal = Invoke-Sm POST "/v1/vault/items/$($item.id)/reveal" @{ password = "password123" } $A
Assert-True ($reveal.Json.secret -eq $secret) "reveal returns the secret after password check"
$leak = (Invoke-Sm POST "/v1/recall" @{ project_id = $project; query = "GitLab 密码" } $A).Text + (Invoke-Sm GET "/v1/sessions/$($r3.Json.session_id)" $null $A).Text
Assert-True (-not $leak.Contains($secret)) "plaintext secret absent from recall and session responses"

# 7. artifact accept -> body readable
$long = "# Redis Streams 学习笔记`n" + ("消费者组、XACK 与 XAUTOCLAIM 的用法。" * 60)
$r4 = Invoke-Sm POST "/v1/ingest" (New-Message $project "assistant" $long) $A
$artPrompt = @($r4.Json.prompts | Where-Object { $_.kind -eq "artifact" })[0]
Assert-True ($null -ne $artPrompt) "artifact candidate offered for a long reply"
$art = (Invoke-Sm POST "/v1/artifacts/candidates/$($artPrompt.candidate_id)/accept" @{} $A).Json
$body = (Invoke-Sm GET "/v1/artifacts/$($art.artifact.id)" $null $A).Json.body
Assert-True ($body -eq $long) "artifact body readable after accept"

# 8. delete A -> A's key stops working, B unaffected
$del = Invoke-Sm DELETE "/v1/account" $null $A @{ "X-Confirm" = "delete" }
Assert-True ($del.Status -eq 204) "delete account A"
Assert-True ((Invoke-Sm GET "/v1/sessions" $null $A).Status -eq 401) "A's data and key are gone"
Assert-True ((Invoke-Sm GET "/v1/sessions" $null $B).Status -eq 200) "B still works"
Write-Host "smoke: all steps passed" -ForegroundColor Green
