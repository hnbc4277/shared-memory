# Shared Memory — OpenClaw（小龙虾）Adapter

TypeScript 插件：OpenClaw hooks ↔ Shared Memory HTTP SDK。

## 一键安装（推荐）

```bash
smctl login
smctl install openclaw    # 或 smctl install xiaolongxia
```

套件复制到 `~/.shared-memory/openclaw/`（含 `INSTALL.txt`）。然后：

```bash
cd ~/.shared-memory/openclaw
npm install
npm run build
```

在 OpenClaw 中启用插件 id `shared-memory`（指向该目录），并设置环境变量：

```bash
SHARED_MEMORY_API_KEY=sm_...
SHARED_MEMORY_API_URL=http://127.0.0.1:18765
# 可选：Web「智能体」注册连接的 UUID
# SHARED_MEMORY_CONNECTION_ID=
```

开发时可直接在仓库内 `adapters/openclaw` 构建（`@sharedmemory/sdk` 为 `file:../../sdk/typescript`）。

## 连接心跳（process_online）

adapter 进程加载期间 `setInterval` 自动 ping（`client=adapter`）：

```text
POST /v1/agents/connections/heartbeat
Authorization: Bearer $SHARED_MEMORY_API_KEY
{"client":"adapter","connection_id":"$SHARED_MEMORY_CONNECTION_ID"}
进程运行期间每 30s 一次
```

OpenClaw 未运行时可用安装目录里的保活脚本（`client=setup`）。`smctl install openclaw` 会写入 `sm-connection-heartbeat.sh` / `.ps1`：

```bash
nohup ./sm-connection-heartbeat.sh >/tmp/sm-connection-heartbeat.log 2>&1 &
```

```powershell
powershell -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File .\sm-connection-heartbeat.ps1
```

计划任务 / systemd --user 可选，安装程序不会自动注册。

## 行为（A+B）

| Hook | Action |
|------|--------|
| `before_prompt_build` | **自动注入**：`recall` → `prependContext` |
| `message_received` / `agent_end` | ingest；尽力填写 provenance |
| `subagent_spawned` | `agent_path=xiaolongxia/<id>` |
| 失败时 | 控制台 Recall Receipt + MCP 补召回 |

无真实 `openclaw` 包时可用 `src/openclaw-plugin-entry.d.ts` stub 做 typecheck。
