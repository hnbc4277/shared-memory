# Shared Memory — DeepSeek Harness Adapter

薄配置包：在 DeepSeek Harness 内复用 **Codex 方言 hooks**（`smhook`）+ **MCP**（`shared-memory-mcp`）。

## 一键安装（推荐）

```bash
smctl login
smctl install deepseek-harness
```

生成目录 `~/.shared-memory/deepseek-harness/`：

| 文件 | 作用 |
|------|------|
| `hooks.json` | Codex 形 hook → `smhook`，`SHARED_MEMORY_AGENT=deepseek-harness` |
| `cordis.snippet.yml` | 合并进项目 `cordis.yml` |
| `INSTALL.txt` | 后续步骤与环境变量模板 |

将 snippet 中的 `plugins:` 并入 harness 项目的 `cordis.yml`，确保 `smhook` / `shared-memory-mcp` 在 PATH（或改绝对路径）。

## 连接心跳（process_online）

会话侧：`shared-memory-mcp` 进程内 ticker（`client=mcp`）。同一契约：

```text
POST /v1/agents/connections/heartbeat
Authorization: Bearer $SHARED_MEMORY_API_KEY
{"client":"adapter","connection_id":"$SHARED_MEMORY_CONNECTION_ID"}
进程运行期间每 30s 一次
```

Harness 未运行时可用安装目录里的保活脚本（`client=setup`）。`smctl install deepseek-harness` 会写入 `sm-connection-heartbeat.sh` / `.ps1`：

```bash
nohup ./sm-connection-heartbeat.sh >/tmp/sm-connection-heartbeat.log 2>&1 &
```

```powershell
powershell -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File .\sm-connection-heartbeat.ps1
```

计划任务 / systemd --user 可选，安装程序不会自动注册。

## 手动安装

```bash
cp adapters/deepseek-harness/cordis.yml.example /path/to/project/cordis.yml
cp adapters/deepseek-harness/hooks.json.example /path/to/project/hooks.json
```

```bash
export SHARED_MEMORY_API_KEY=sm_...
export SHARED_MEMORY_API_URL=http://127.0.0.1:18765
```

## Agent 命名

规格 `agent=deepseek-harness`。`smhook` 通过环境变量 `SHARED_MEMORY_AGENT` 覆盖上报名（hooks 已设置）；MCP 工具与 agent 无关。

## 行为（A+B）

| Path | Role |
|------|------|
| `dsh-hooks-codex` + `smhook` | **自动注入** `additionalContext` + ingest |
| `dsh-mcp-client` + MCP | 模型可调用 `memory_recall` 等 |
| Receipt | 控制台「密钥」页可审计是否已召回 |
