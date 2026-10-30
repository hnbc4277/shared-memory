# Shared Memory

**当前版本：`v0.1.0`**

自托管的**跨智能体共享记忆**：把 Cursor、豆包等工作区里的对话与偏好写入你自己的服务，再在其它客户端里召回，而不是锁死在某一家厂商的云端记忆里。

> 适合个人/小团队私有部署。仓库公开 ≠ 有人替你运维；线上 API、数据库与密钥仍需自行维护。

## 能做什么

- **写入与召回**：`memory_ingest` / `memory_recall`（及知识库、成稿、Vault 等能力，见控制台与 API 文档）
- **Web 控制台**：连接实例、会话历史、召回回执、密钥与策略
- **连接归属**：按「智能体连接」区分多台设备；专用 API Key 可自动归属，无需每次填 `connection_id`
- **客户端适配**：本机安装脚本 + MCP / Skill（因产品而异）

## 版本与已调试客户端

| 项目 | 版本 / 范围 | 状态 |
|------|----------------|------|
| **Shared Memory（本仓库）** | **`v0.1.0`** | 当前发布版本 |
| **Cursor** | **Windows** 桌面 **`3.27.14`** | 已调试通过（Hook + MCP + 配置脚本） |
| **豆包** | **Windows** 桌面 **`2.29.10`**（工作 / Agent 模式） | 已调试通过（MCP 连接器 + Skill） |

说明：

- `v0.1.0` 阶段**仅上述两个 Windows 客户端**完成联调；其它系统或其它智能体**不在本版本承诺范围内**。
- 豆包：**不支持**手机 App；桌面也需模型主动调用工具才会写入平台（无静默 hook）。
- 客户端小版本升级后安装路径 / MCP 表单可能变化；若与上表不一致，请先对照本机版本再反馈。

### 适配器材料（未在本版本承诺联调完成）

| 客户端 | 说明 |
|--------|------|
| Hermes / OpenClaw / DeepSeek Harness 等 | 仓库内有 `adapters/` 草稿，**`v0.1.0` 未做完整联调验收** |

## 架构一览

```text
客户端 (Cursor / 豆包桌面 / …)
    │  MCP 或 Hook / smctl
    ▼
API (:18080)  ←→  Postgres / Redis / MinIO / OpenSearch / Qdrant
    │
Worker（索引、派生、清理等）
    │
Web 控制台（Vite，可反代或由 API 托管静态资源）
```

## 快速开始（本地）

**依赖**：Docker、Go 1.25+、Node.js（前端）、（可选）Make。

```bash
# 1. 环境变量
cp .env.example .env
# 按需改 CORS_ORIGINS、PUBLIC_API_URL 等；切勿把真实 .env 提交进 Git

# 2. 基础设施 + 应用（示例）
docker compose --profile app up -d --build
# 或：make app

# 3. Web 开发服务器
cd web && npm install && npm run dev
# 默认 http://127.0.0.1:15173 ；API 默认 http://127.0.0.1:18080
```

更完整的部署、迁移与上线清单见 **[docs/公开/DEPLOY.md](docs/公开/DEPLOY.md)**。  
操作说明见 **[docs/公开/manual/OPERATIONS-MANUAL.md](docs/公开/manual/OPERATIONS-MANUAL.md)**（及同目录图文手册）。  
文档公开范围说明见 **[docs/README.md](docs/README.md)**。

### 客户端安装（示例）

```bash
# 登录后安装适配器材料到 ~/.shared-memory/…
smctl login
smctl install cursor    # 或 doubao / hermes / openclaw / …
```

- Cursor：按控制台「配置步骤」或 `adapters` / 安装目录说明完成 hooks 与 MCP  
- 豆包：在桌面「自定义连接器」中**逐项填写** STDIO 命令与环境变量（不要把整段 JSON 贴进「命令」），并启用 `shared-memory` Skill  

适配器说明：

- [adapters/doubao](adapters/doubao/README.md)
- [adapters/hermes](adapters/hermes/README.md)
- [adapters/openclaw](adapters/openclaw/README.md)
- [adapters/deepseek-harness](adapters/deepseek-harness/README.md)

## 配置注意

| 变量 | 作用 |
|------|------|
| `POSTGRES_DSN` | 主库（必填） |
| `CORS_ORIGINS` | 控制台 Origin 白名单 |
| `PUBLIC_API_URL` | 写入客户端配置脚本的外网/可达 API 根（公司电脑常用） |
| `SHARED_MEMORY_WEB_URL` | 设备码等链接用的控制台地址 |
| `WEB_STATIC_DIR` | 由 API 托管 `web/dist` 时指向构建产物 |
| `VAULT_MASTER_KEY` | Vault 主密钥，务必离线保管 |

客户端侧常见环境变量：`SHARED_MEMORY_API_URL`、`SHARED_MEMORY_API_KEY`、`SHARED_MEMORY_AGENT`、`SHARED_MEMORY_CONNECTION_ID`。

## 仓库结构（节选）

```text
cmd/           API、worker、mcp、smctl 等入口
internal/      业务与 HTTP
adapters/      各智能体安装说明与模板
web/           控制台前端
migrations/    Postgres 迁移
docs/          部署、API、规格与手册
scripts/       smoke / 运维脚本
```

## 开发与验收

```bash
go test ./...
# 集成（需测试库等环境）：
# go test -tags=integration ./internal/agents/ ./internal/recall/ ./internal/httpapi/ ...

# Windows 冒烟示例
powershell -File scripts/e2e-smoke.ps1
```

API 索引：[docs/公开/API.md](docs/公开/API.md) · OpenAPI：[docs/公开/openapi.yaml](docs/公开/openapi.yaml)

## 安全与公开仓库

发布到 GitHub 前请确认：

- [ ] 未提交 `.env`、真实 API Key、证书私钥、生产备份
- [ ] `.gitignore` 已覆盖 `bin/`、`*.exe`、`.env`、`.worktrees/` 等
- [ ] 示例配置只用占位符（见 `.env.example`）
- [ ] 根目录已有 [LICENSE](LICENSE)（MIT）

**不要**把生产 `PUBLIC_API_URL` 上的专用密钥或 Vault 主密钥写进 README / Issue。

## 维护与贡献

- 本项目以**自托管**为主；作者/团队负责自己的部署与升级。
- 欢迎 Issue 描述缺陷或需求；功能 PR 请尽量附测试与简短说明。
- 适配器依赖第三方客户端（Cursor、豆包等）的扩展能力，对方改版可能导致安装路径或 MCP 表单变化，属预期风险。
- **不保证**有人长期跟进你的 fork；若仅自用，可保持私有仓库或缩小承诺范围。

## 文档

| 文档 | 内容 |
|------|------|
| [docs/公开/DEPLOY.md](docs/公开/DEPLOY.md) | 部署、迁移、自托管清单 |
| [docs/公开/API.md](docs/公开/API.md) | HTTP API 索引 |
| [docs/公开/manual/](docs/公开/manual/) | 操作手册 |
| [docs/README.md](docs/README.md) | 文档目录索引 |

## License

本项目采用 [MIT License](LICENSE)。
