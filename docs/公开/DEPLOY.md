# 部署与迁移

本地/Compose 环境部署 Shared Memory API、Worker 与 Web 控制台的检查清单。

## 前置

1. 仓库根目录：`docker compose --profile app up -d --build`（或 `make app`）。
2. API 默认：`http://127.0.0.1:18080`；Web dev：`cd web && npm run dev` → `http://127.0.0.1:15173`（登录后默认 **总览** Tab，顶栏 **Ctrl+K** 跳模块）。
3. 复制 `.env.example` 为 `.env`，至少配置 `POSTGRES_DSN`、`CORS_ORIGINS`（含控制台来源），以及对外访问时的 **`PUBLIC_API_URL`**（见下节）。
4. 使用说明：[操作手册](manual/OPERATIONS-MANUAL.md) / 浏览器 [图文手册](manual/图文手册.html)。

## `PUBLIC_API_URL`（客户端可达的 API 根地址）

配置脚本与控制台「配置步骤」里的 **域名 + 端口** 只认 API 进程读取的 `.env`，**不要**写死在前端或仓库示例里的个人域名。

| 项 | 说明 |
|----|------|
| **配置位置** | 仓库根目录 `.env`（与 `HTTP_ADDR`、`POSTGRES_DSN` 同文件） |
| **变量名** | `PUBLIC_API_URL` |
| **含义** | 公司电脑 / 豆包 / Cursor MCP **能访问到的** API 根，含协议与端口，无尾斜杠 |
| **示例** | 本机同机：`http://127.0.0.1:18080`；外网或端口映射：`http://your.domain:28080` 或 `https://api.example.com` |
| **谁在用** | ① 创建连接并勾选专用密钥时，写入下载包内 setup 脚本的 `SHARED_MEMORY_API_URL` / DownloadBase；② `GET /v1/me` 的 `public_api_url`，供控制台豆包/Cursor 配置步骤展示 |
| **留空时** | 生成脚本回退为 `http://127.0.0.1:18080`；控制台引导则回退为当前 `VITE_API_BASE` |

```env
# .env
HTTP_ADDR=:18080
PUBLIC_API_URL=http://your.domain:28080
```

修改后需**重启 API**；已下载的旧 zip / 已填进豆包连接器的地址不会自动更新，请重新下载配置脚本或改客户端环境变量。

本地仅本机调试、客户端也在同一台机器时，可设：

```env
PUBLIC_API_URL=http://127.0.0.1:18080
```

## 数据库迁移

迁移在 **API 启动时自动执行**（`internal/app` → `db.Migrate` → `migrations/` 下所有 `*.up.sql`，记录于 `schema_migrations`）。

智能体连接与归属相关版本（须按序号存在，不可跳过）：

| 版本文件 | 内容 |
|----------|------|
| `0011_agent_connections.up.sql` | `agent_connections` 表 |
| `0012_session_connection_id.up.sql` | `sessions.connection_id` |
| `0013_connection_attribution.up.sql` | messages / memories / recall_receipts 的 connection 字段 |

手动验证（Postgres 已起、DSN 正确）：

```powershell
go run ./cmd/api
# 日志无 migrate 错误；或 integration 测试会自动 migrate
```

集成测试使用 `testutil.TestPool`，同样会对测试库跑全量 up 迁移。

生产/预发可核对（Postgres）：

```sql
SELECT version FROM schema_migrations ORDER BY version;
-- 应包含 0011、0012、0013（文件名前缀与 version 列一致）
```

## 索引与 `source_connection_id`

升级后若 OpenSearch/Qdrant 中仍是旧 mapping（无 `source_connection_id` 字段），connection 维度的**检索预过滤**可能不完整。建议在维护窗口：

1. 确认 API 与 Worker 已部署新版本（artifact/memory 索引写入含 `source_connection_id`）。
2. 触发全量重投影：运维入口或 `Reindexer`/`memory.ReindexAll`（见 `internal/reindex`、`docs` 内 reindex 说明）。
3. 验收：双 connection 下 recall 不串命中（`e2e-smoke.ps1` Recall 隔离段或 integration `TestRecall_FiltersHitsByConnection`）。

## 回滚

各迁移配有 `.down.sql`。**仅建议在非生产**执行 down；`0013` down 会删除 attribution 列，不恢复 OpenSearch/Qdrant 投影内容。

## 自托管服务器（非 GitHub）

1. **构建**：`go build -o shared-memory-api ./cmd/api`（worker/mcp 同理）；`cd web && npm run sync-docs && npm run build`。
2. **同源或分域**：
   - **推荐**：Nginx/Caddy 反代，`/api` → API、`/` → `web/dist`；或设置 `WEB_STATIC_DIR=web/dist` 由 API 托管静态页。
   - **分域**：API 的 `CORS_ORIGINS` 必须列出控制台完整 origin（不能 `*`），浏览器用 Cookie 登录时需 `Allow-Credentials`（已内置）。
3. **TLS**：公网务必 HTTPS；Cookie `Secure` 在 TLS 或 `X-Forwarded-Proto: https` 下自动开启。
4. **防火墙**：仅暴露 443（或内网 VPN）；Postgres/Redis/MinIO **不要**对公网开放。
5. **备份**：定期 `pg_dump`；MinIO bucket 快照；记录 `.env` 与 `VAULT_MASTER_KEY` 离线保管。
6. **版本**：部署前 `git tag`；回滚 = 旧二进制 + 仅在 down 迁移允许时使用 `.down.sql`。
7. **验收**：
   - 一键（Windows）：`powershell -File scripts/ops/selfhosted-release.ps1 -Reindex -Smoke -Tag v0.1.0 -Api https://你的域名`
   - 或：`API=https://你的域名 bash scripts/e2e-smoke.sh`（需 `jq` 或 `python3`）
   - 迁移核对：`psql "$POSTGRES_DSN" -f scripts/ops/verify-migrations.sql`
   - 重索引：`bash scripts/ops/reindex.sh`
   - 备份示例：`bash scripts/ops/backup-postgres.example.sh`
   - 验收说明：`docs/公开/ops/ACCEPTANCE.md`；OpenAPI：`docs/公开/openapi.yaml`

## 上线检查清单

- [ ] `schema_migrations` 含 0011–0013
- [ ] `CORS_ORIGINS` 含控制台来源；邮箱登录需 `Access-Control-Allow-Credentials`（见 `web/README.md`）
- [ ] `PUBLIC_API_URL` 为客户端真实可达地址（公司机 / 外网不要用仅本机可访问的 `127.0.0.1`，除非客户端也在同机）
- [ ] Redis 可用时 `POST /v1/auth/login|register` 受 IP 窗口限流（与账号热路径共用 `RateLimit` 配置）
- [ ] 路由索引：`docs/公开/API.md`
- [ ] `SHARED_MEMORY_WEB_URL` / 设备码链接正确
- [ ] `npm run sync-docs` 后构建 `web/dist`（或设置 `WEB_STATIC_DIR`）；`web/public/docs/manual/` 含图文手册（源在 `docs/公开/manual/`）
- [ ] `go test ./...` 与 `go test -tags=integration ./internal/agents/ ./internal/recall/ ./internal/httpapi/ ...`
- [ ] `powershell -File scripts/e2e-smoke.ps1`（或 Actions 手动 `smoke-compose`）

## 发布后验收

```powershell
curl http://127.0.0.1:18080/healthz
go test ./...
go test -tags=integration ./internal/agents/ ./internal/recall/
cd web; npm run build
powershell -File scripts/e2e-smoke.ps1
# Linux/macOS（精简段）: API=... bash scripts/e2e-smoke.sh
```

## CI

见 `.github/workflows/ci.yml`：`go test ./...`、`node scripts/check-web-docs-sync.mjs`（`web/public` 与源文档一致）、`web` 的 `npm run build`、`go-integration`（Postgres）。完整 smoke：本地 `e2e-smoke.ps1` / `e2e-smoke.sh`；CI 可在 Actions 页 **手动运行** `smoke-compose` job（需 Docker，消耗 runner 分钟，**不依赖第三方付费 SaaS**）。

**费用：** 自托管 Compose 无强制云订阅；私有仓库 GitHub Actions 可能计分钟；可选 `LLM_DERIVER_*` / `VISION_*` / 云 Embedder 按云厂商计费。

**限流：** Redis 固定窗口（默认 120 次/分钟/账号）；`ingest`、`recall`、`agents/connections` 创建、`vault/items`、`artifacts` 创建等热路径已接入；Redis 不可用时会 fail-open。

## 环境变量（设备码与其它）

| 变量 | 说明 |
|------|------|
| `PUBLIC_API_URL` | 见上文「客户端可达的 API 根地址」 |
| `SHARED_MEMORY_WEB_URL` | 设备验证链接用的控制台 URL（默认常误为 `:5173`；Vite dev 常用 `http://127.0.0.1:15173`），须与实际控制台一致 |

设备验证链接使用 `SHARED_MEMORY_WEB_URL`。在 `.env` 中与控制台实际 URL 一致。

## Web 控制台（生产）

API **默认不托管** 前端静态资源。推荐：

1. **开发**：`cd web && npm run dev`（`:15173`），`VITE_API_BASE` 指向 API；API 的 `CORS_ORIGINS` 包含控制台来源。
2. **生产构建**：`cd web && npm run sync-docs && npm run build`，将 `web/dist/` 交给 Nginx/Caddy/对象存储静态站；同一域名反代 `/v1` 到 Go API 时可设 `VITE_API_BASE=/` 或完整 API 域名。
3. **单进程托管（可选）**：在 `.env` 设置 `WEB_STATIC_DIR=web/dist`，API 会托管控制台静态资源（`/v1/*` 仍为 API）。
4. **文档链**：未设置 `VITE_REPO_DOCS_BASE` 时使用 `web/public/` 副本；发布前执行 `npm run sync-docs`（见 `web/package.json`）或根目录 `scripts/sync-web-docs.ps1`。控制台改版后可选 `scripts/capture-manual-screenshots.ps1` 更新手册 PNG。
