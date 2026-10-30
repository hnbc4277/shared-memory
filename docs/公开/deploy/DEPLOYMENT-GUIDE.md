# Shared Memory 部署指南（自托管 · 新手向）

本文从**零**说明如何在单机（Windows / Linux / macOS）部署完整栈：**依赖服务 + API + Worker + Web**。精简清单见 [DEPLOY.md](../DEPLOY.md)。

---

## 1. 你需要准备什么

| 项目 | 最低建议 |
|------|----------|
| CPU / 内存 | 4 核 / 8 GB（OpenSearch + Embedder 较吃内存） |
| 磁盘 | 20 GB+（模型缓存、MinIO、Postgres） |
| 软件 | Docker Desktop 或 Docker Engine + Compose v2 |
| 开发构建（可选） | Go 1.22+、Node 20+（仅当你不用 Docker 跑 API 时） |
| 网络 | 首次拉 embedder 模型需访问 HuggingFace（Compose 内可配镜像） |

**端口占用（默认 Compose）：**

| 服务 | 宿主机端口 |
|------|------------|
| Postgres | 15432 |
| Redis | 16379 |
| MinIO API / Console | 19000 / 19001 |
| OpenSearch | 19200 |
| Qdrant | 16333 |
| Embedder | 18081 |
| API | 18080 |
| Web dev (Vite) | 15173 |

---

## 2. 获取代码与环境文件

```powershell
cd D:\projectsAll\cursorProjects\sharedMemory
Copy-Item .env.example .env
```

用编辑器打开 `.env`，至少确认：

```env
POSTGRES_DSN=postgres://sm:sm@127.0.0.1:15432/sharedmemory?sslmode=disable
REDIS_ADDR=127.0.0.1:16379
MINIO_ENDPOINT=127.0.0.1:19000
MINIO_ACCESS_KEY=smminio
MINIO_SECRET_KEY=smminio_secret
MINIO_BUCKET=shared-memory
OPENSEARCH_URL=http://127.0.0.1:19200
QDRANT_URL=http://127.0.0.1:16333
EMBEDDER_URL=http://127.0.0.1:18081
VAULT_MASTER_KEY=0123456789abcdef0123456789abcdef
CORS_ORIGINS=http://127.0.0.1:15173,http://localhost:15173
# 客户端（Cursor / 豆包）能访问到的 API 根；写入配置脚本，勿依赖前端写死
PUBLIC_API_URL=http://127.0.0.1:18080
```

> **生产务必**更换 `VAULT_MASTER_KEY`（32 字节 hex）与 MinIO 密码，并妥善离线备份。  
> 公司电脑或外网客户端请把 `PUBLIC_API_URL` 改成映射后的域名+端口（如 `http://your.domain:28080`），改完重启 API，并重新下载配置脚本。详见 [DEPLOY.md · PUBLIC_API_URL](../DEPLOY.md#public_api_url客户端可达的-api-根地址)。

Compose 内 API/Worker 使用 `deploy/compose.env`（与上述值对齐）；本地 `go run` 时读 `.env`（见 `SHARED_MEMORY_ENV`）。

---

## 3. 启动依赖与应用（推荐 Docker）

### 3.1 仅基础设施

```powershell
docker compose up -d
docker compose ps
```

等待 `postgres`、`redis` 为 healthy。首次会下载 OpenSearch、Qdrant 镜像并拉取 embedding 模型（`embedder-model` 任务）。

### 3.2 启动 API + Worker（app profile）

```powershell
docker compose --profile app up -d --build
```

**重要：** 每次拉取含智能体连接等新路由的代码后，必须 **`--build`**，否则 Smoke 会出现 `GET /v1/agents` 404（见 [验收清单](../ops/ACCEPTANCE.md)）。

### 3.3 验证 API

```powershell
curl.exe http://127.0.0.1:18080/healthz
# 期望: ok

curl.exe -i http://127.0.0.1:18080/v1/agents -H "Authorization: Bearer 无效"
# 期望: 401 unauthorized（而不是 404）
```

### 3.4 验证迁移

```powershell
# 需 psql 客户端
psql "postgres://sm:sm@127.0.0.1:15432/sharedmemory?sslmode=disable" -f scripts/ops/verify-migrations.sql
```

或查看 API 启动日志无 `migrate` 错误。

---

## 4. Web 控制台

### 4.1 开发模式（推荐调试）

```powershell
cd web
npm install
npm run dev
```

浏览器打开 `http://127.0.0.1:15173`。默认 `VITE_API_BASE` 指向 `http://127.0.0.1:18080`（见 `web/vite.config.ts` / `.env`）。

登录方式：

- **API Key：** 注册后在「密钥与回执」页创建，粘贴到登录框（分段 **API 密钥**）。
- **Cookie：** 邮箱注册/登录（需 CORS 与 `credentials: include`）。

登录成功后默认进入 **总览**（`#overview`）。顶栏 **Ctrl+K** 可跳转任意模块；分步说明见 [操作手册](../manual/OPERATIONS-MANUAL.md) 第 2 节。浏览器看图：`http://127.0.0.1:15173/docs/manual/图文手册.html`（需 dev/preview 且已 `sync-docs`）。

### 4.2 生产静态资源

```powershell
cd web
npm run sync-docs    # 同步 adapters/docs 到 public
npm run build        # 输出 web/dist
```

两种托管方式：

**A. 反代分离（推荐）**

- Nginx：`/` → `web/dist`，`/v1` → `http://127.0.0.1:18080`
- 构建时 `VITE_API_BASE=/` 或完整 API 域名

**B. API 单进程托管**

`.env` 增加：

```env
WEB_STATIC_DIR=web/dist
```

重启 API 后同一端口提供 UI + API。

### 4.3 控制台文档与手册配图

- 用户文档源：`docs/公开/manual/`。浏览器可访问的 [图文手册.html](../manual/图文手册.html) 副本在 `web/public/docs/manual/`（改版时从 `docs/公开/manual/` 同步该目录）。
- `npm run sync-docs` 同步的是适配器/spec 等到 `web/public/`（见 `scripts/sync-web-docs.ps1`），**不含**整本手册 Markdown。
- UI 改版后重生成 Playwright 配图（需 API `healthz` 与 preview）：

```powershell
powershell -File scripts/capture-manual-screenshots.ps1
```

详见 [manual/images/README.md](../manual/images/README.md)（含 `01-console-shell` 总览、`01b-cmd-palette` 等）。

---

## 5. 不用 Docker 跑 API/Worker（进阶）

前提：Compose 已起 Postgres/Redis/MinIO/OS/Qdrant/Embedder。

```powershell
go run ./cmd/api
# 另开终端
go run ./cmd/worker
```

环境变量从 `.env` 加载（`internal/config`）。Windows 可：

```powershell
$env:SHARED_MEMORY_ENV=".env"
go run ./cmd/api
```

---

## 6. 生产 HTTPS 与反向代理

### 6.1 Caddy 示例（概念）

```caddy
your.domain.com {
  reverse_proxy /v1/* 127.0.0.1:18080
  reverse_proxy /healthz 127.0.0.1:18080
  root * /var/www/sharedmemory/dist
  file_server
}
```

### 6.2 必配项

| 变量 | 说明 |
|------|------|
| `CORS_ORIGINS` | 控制台完整 origin，如 `https://your.domain.com` |
| `PUBLIC_API_URL` | 客户端可达的 API 根（含端口）；写入 setup 脚本与 `GET /v1/me` 的 `public_api_url` |
| `SHARED_MEMORY_WEB_URL` | 设备码邮件/页面链接，与控制台 URL 一致 |
| TLS | 公网必须 HTTPS；Cookie `Secure` 在 `X-Forwarded-Proto: https` 下生效 |

### 6.3 防火墙

- 对外只开 **443**（或 VPN 内网）
- **不要**把 15432、16379、19000、19200、16333 暴露到公网

---

## 7. 可选能力

| 变量 | 作用 |
|------|------|
| `LLM_DERIVER_*` | 更强语义 derive；失败回退启发式 |
| `VISION_*` | 图片附件多模态；账号策略可关 cloud_vision |
| `RATE_LIMIT_*` / `AUTH_*` | 限流阈值 |
| `RECALL_RECEIPT_RETENTION_DAYS` | Receipt 保留 |
| `OCR_LANGS` | Tesseract 语言（Worker 镜像内置） |

---

## 8. MCP 与 smctl（客户端）

```powershell
go build -o smctl.exe ./cmd/smctl
go build -o mcp-server.exe ./cmd/mcp
```

`smctl login` / `smctl install` 将适配器写入 Cursor 等配置；详见 [操作手册](../manual/OPERATIONS-MANUAL.md)。

---

## 9. 发布后验收

```powershell
go test ./...
cd web; npm test; npm run build
powershell -File scripts/e2e-smoke.ps1 -Api http://127.0.0.1:18080
```

Linux：

```bash
API=http://127.0.0.1:18080 bash scripts/e2e-smoke.sh
```

一键（Windows 运维脚本）：

```powershell
powershell -File scripts/ops/selfhosted-release.ps1 -Reindex -Smoke -Tag v0.1.0 -Api http://127.0.0.1:18080
```

说明见 [docs/公开/ops/ACCEPTANCE.md](../ops/ACCEPTANCE.md)。

---

## 10. 常见问题

| 现象 | 处理 |
|------|------|
| `GET /v1/agents` 404 | 重建 `app` 镜像：`docker compose --profile app up -d --build` |
| Web 登录 CORS 错误 | `CORS_ORIGINS` 缺少控制台 origin；分域需 Cookie 模式 |
| Recall 无命中 | 确认 worker 在跑；ingest 后等 30–45s；查 OpenSearch/Qdrant 健康 |
| KB 一直 parsing | worker 日志；MinIO 可读；embedder 就绪 |
| migrate 失败 | 查 `schema_migrations` 与 SQL 错误；不要手工改序号 |

---

## 11. 备份（最小）

```bash
bash scripts/ops/backup-postgres.example.sh
```

MinIO：对 `shared-memory` bucket 做快照或 `mc mirror`。`.env` 与 `VAULT_MASTER_KEY` **离线加密保存**。

---

## 12. 相关文档

- [系统设计](../design/SYSTEM-DESIGN.md)
- [迁移指南](../migration/MIGRATION-GUIDE.md)
- [操作手册](../manual/OPERATIONS-MANUAL.md) · [图文手册](../manual/图文手册.html) · [FEATURE-INDEX](../manual/FEATURE-INDEX.md)
