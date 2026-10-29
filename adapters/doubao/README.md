# Shared Memory — 豆包（Doubao）Adapter

方案 **C**：官方扩展点组合 —— **MCP 连接器**（读写）+ **Skill**（何时召回/写入）。无静默 hook，无 CDP/逆向。

## 一键安装（推荐）

```bash
smctl login
smctl install doubao
```

生成目录 `~/.shared-memory/doubao/`：

| 文件 | 作用 |
|------|------|
| `mcp.connector.json` | 填好路径/凭证的 MCP 连接器片段（导入豆包工作自定义连接器） |
| `mcp.connector.example.json` | 示例模板 |
| `SKILL.md` | 共享记忆技能（上传到「插件·技能·伙伴 → 技能」） |
| `INSTALL.txt` | 后续步骤与环境变量 |

确保 `shared-memory-mcp` 在 PATH，或把 `mcp.connector.json` 里的 `command` 改成绝对路径。

## 在豆包工作中启用

1. **连接器**：打开「插件·技能·伙伴 → 连接器 / 自定义 MCP」，新建 STDIO 连接器，按 `mcp.connector.json` 填写 `command` 与 `env`（字段名以豆包客户端 UI 为准；核心是启动 `shared-memory-mcp` 并注入下表环境变量）。
2. **技能**：`smctl install doubao` 会尽量自动写入豆包本机 `.user_skills/shared-memory/SKILL.md`（非官方路径，客户端改版可能失效）。若未自动成功，再手动上传本目录 `SKILL.md`。新开任务或重启客户端后生效。
3. **连接归属（可选）**：在 Web「智能体」添加产品「豆包」，勾选专用 API 密钥；或把 `SHARED_MEMORY_CONNECTION_ID` 写入连接器环境变量。

## 环境变量

| 变量 | 说明 |
|------|------|
| `SHARED_MEMORY_API_URL` | API 根，默认 `http://127.0.0.1:18765` |
| `SHARED_MEMORY_API_KEY` | `sm_...`（全局或连接专用） |
| `SHARED_MEMORY_AGENT` | 固定 `doubao` |
| `SHARED_MEMORY_CONNECTION_ID` | 可选；专用 Key 时可留空 |

## 连接心跳（process_online）

会话侧：`shared-memory-mcp` 进程内 ticker（`client=mcp`）。同一契约：

```text
POST /v1/agents/connections/heartbeat
Authorization: Bearer $SHARED_MEMORY_API_KEY
{"client":"adapter","connection_id":"$SHARED_MEMORY_CONNECTION_ID"}
进程运行期间每 30s 一次
```

MCP 未运行时可用安装目录里的保活脚本（`client=setup`）。`smctl install doubao` 会写入 `sm-connection-heartbeat.sh` / `.ps1`。

## Agent 命名

规格 `agent=doubao`。控制台「智能体」分组展示名为「豆包」。

## 行为（C）

| Path | Role |
|------|------|
| 自定义 MCP 连接器 + `shared-memory-mcp` | 模型可调用 `memory_recall` / `memory_ingest` 等 |
| `SKILL.md` | 提高「先召回、后写入」的调用率（非静默） |
| Receipt | 控制台「密钥」页可审计是否已召回 |

豆包自带服务端记忆与 Shared Memory **不互通**；本适配器只走你们自己的 API。
