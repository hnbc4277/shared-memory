# Shared Memory 迁移指南（Schema + 数据 + 索引）

适用于：**版本升级**、**换机恢复**、**从旧库导入**。新手请先读 [部署指南](../deploy/DEPLOYMENT-GUIDE.md) 再起库。

---

## 1. 迁移机制概览

| 项 | 说明 |
|----|------|
| 位置 | 仓库 `migrations/` |
| 命名 | `{version}_{name}.up.sql` / `.down.sql` |
| 执行时机 | **API 进程启动**时 `db.Migrate` 自动应用尚未执行的 `.up.sql` |
| 记录表 | `schema_migrations(version)` |
| 测试库 | `go test -tags=integration` 通过 `testutil.TestPool` 同样跑全量 up |

**原则：**

1. **不要跳过版本号**（0011 必须在 0012 之前）。
2. **生产环境慎用 down**——down 只删列/表，不恢复 MinIO/OpenSearch/Qdrant 内容。
3. **Schema 升级 ≠ 搜索索引升级**——新增字段（如 `source_connection_id`）后需 **Reindex**。

---

## 2. 版本清单（0001 → 0013）

### 0001_init — 基线

- **表：** `accounts`、`api_keys`、`projects`、`account_policies`、`sessions`、`messages`、`events`、`attachments`、`memories`、`profiles`、`audit_logs`
- **数据影响：** 新库空表；无破坏性
- **回滚 down：** 删除上述表（仅空库/测试）

### 0002_message_provenance — 溯源与索引

- **变更：** messages/sessions 上 provenance 相关列与索引；`messages(session_id, seq)` 唯一
- **数据：** 旧消息若缺 skills 等，需应用层保证新 ingest 合规
- **运维：** 大表加索引可能锁表，低峰执行

### 0003_vault — 凭证库

- **表：** `vault_candidates`、`vault_items`
- **数据：** 新功能，无历史迁移

### 0004_artifacts — 成稿库

- **表：** `artifact_candidates`、`artifacts`、`artifact_versions`

### 0005_index_state — 向量集合状态

- **表：** `index_state`（记录 embedder 集合是否全量 backfill）
- **数据迁移：** 换 embedder 模型时会新 collection，worker 根据此表决定是否全量 reindex

### 0006_m3 — 设备码与外部会话

- **表：** `devices`、`device_codes`
- **索引：** `sessions` 上 `external_session_id` 唯一（partial）

### 0007_design_gaps — 隔离区、tool_calls、实体、Web 会话

- **表：** `ingest_quarantine`、`tool_calls`、`entity_aliases`、`web_sessions`
- **说明：** History 页 tool_calls 展示依赖 `tool_calls` 表

### 0008_recall_receipts — Recall 审计

- **表：** `recall_receipts` 及索引
- **数据：** 新 recall 开始写 receipt；Worker 按 `RECALL_RECEIPT_RETENTION_DAYS` 清理

### 0009_cloud_vision — 云视觉策略位

- **变更：** `account_policies` 增加 cloud vision 相关字段（见 SQL）
- **数据：** 默认关闭云视觉，按需开启

### 0010_knowledge_base — 知识库

- **表：** `kb_documents`、`kb_chunks`
- **数据：** 上传文档后由 worker `kb_parse` 写 chunk；**无自动从 memories 导入**

### 0011_agent_connections — 智能体连接

- **表：** `agent_connections`（auto / registered / device 等 kind）
- **数据：** 升级后**不会**自动为历史 session 创建 connection；首次 ingest 或用户注册时创建

### 0012_session_connection_id — 会话归属

- **变更：** `sessions.connection_id` 可空 FK → `agent_connections`
- **数据迁移建议：**
  - 已有 session 保持 `NULL`，直到用户 **claim-history** 或新 ingest 绑定
  - 无需批量 SQL，除非你们自有脚本按规则回填

### 0013_connection_attribution — 消息/记忆/Receipt 归属

- **变更：** `messages.connection_id`、`memories.source_connection_id`、`recall_receipts.connection_id` 等 + 索引
- **索引投影：** 新写入带 connection 字段；**旧 OpenSearch/Qdrant 文档无该字段 → 必须 Reindex**

---

## 3. 升级步骤（标准）

### 3.1 维护窗口前

1. **备份 Postgres**（见 `scripts/ops/backup-postgres.example.sh`）
2. 记录 MinIO bucket 大小；可选快照
3. 记录当前版本：`SELECT version FROM schema_migrations ORDER BY version;`
4. 记录当前 Git tag / 镜像 digest

### 3.2 执行升级

```powershell
# 1. 拉新代码
git pull   # 或替换二进制

# 2. 重建并重启（Docker）
docker compose --profile app up -d --build

# 3. 看 API 日志确认 migrate 成功

# 4. 验证 schema
psql "$POSTGRES_DSN" -f scripts/ops/verify-migrations.sql
```

### 3.3 索引与数据投影（0013 及 embedder 变更后必做）

```powershell
# 确保 .env 中 OS/Qdrant/Embedder 可达
$env:SHARED_MEMORY_ENV=".env"
go run ./cmd/worker -reindex
# 或
bash scripts/ops/reindex.sh
```

**Reindex 做什么：**

- 按 Postgres 中 memories、artifacts（及 KB 等相关集合）**重新写入** OpenSearch/Qdrant
- 更新 `index_state`
- claim-history 后也会对单 connection 异步 reindex（见 `handlers_agents.go`）

### 3.4 验收

```powershell
powershell -File scripts/e2e-smoke.ps1
go test -tags=integration ./internal/agents/
# recall 集成需 OPENSEARCH_URL、QDRANT_URL 指向 Compose
```

---

## 4. 数据迁移场景

### 4.1 同版本换机（Postgres + MinIO 迁移）

1. 源机：`pg_dump -Fc` 全库
2. 源机：MinIO `mc mirror` 或卷打包 `sharedmemory_minio_data`
3. 目标机：起 Compose **同版本**迁移文件
4. 恢复：`pg_restore` 到新 Postgres
5. 恢复 MinIO 数据卷或 mirror 回 bucket
6. 复制 `.env`（**尤其 `VAULT_MASTER_KEY` 必须一致**，否则 Vault 不可解密）
7. 启动 API/Worker；**通常不必** reindex（若索引卷一并迁移）

### 4.2 仅 Postgres 恢复、索引为空

1. 恢复 PG 后启动 Worker
2. 执行 **全量 reindex**（§3.3）
3. KB 文档若 `status=ready` 但 Qdrant 无向量，可对单文档 `POST .../reparse`

### 4.3 从「无 agent_connections」旧版升到 0011+

1. 自动 up SQL
2. 控制台「智能体」为空直到有新 ingest
3. 用户可对 auto 连接 **upgrade** 或 **claim-history**
4. 若需 Recall 按 connection 隔离，完成 0013 后 **reindex**

### 4.4 账号合并 / 导入第三方会话

**产品未提供一键导入器。** 需要自写脚本：

- 调用 `POST /v1/ingest` 逐条写入（遵守 ValidateIngest）
- 或 SQL 直插（**不推荐**——绕过 quarantine、队列、对象存储一致性）

### 4.5 删号与 GDPR

`DELETE /v1/account` 级联删除 PG 行；Deleter 清理 MinIO 前缀；索引靠 worker/retention 或 reindex 清理。**删号不可恢复**。

---

## 5. 回滚策略

| 场景 | 建议 |
|------|------|
| 新二进制有 bug | 回滚**二进制/镜像**到旧 tag；**若 up 迁移已执行**，不要 down，除非在测试库验证过 down |
| 误执行 down | 可能丢列/丢表；从 **pg_dump 备份** 恢复 |
| 索引错误 | 不要 down SQL；执行 reindex 或删 Qdrant collection 后 reindex |

**0013 down.sql：** 删除 attribution 列；**不会**从搜索引擎删除旧字段文档——再次 up 后仍建议 reindex。

---

## 6. 核对命令速查

```sql
-- 已应用迁移
SELECT version FROM schema_migrations ORDER BY version;

-- 连接表是否存在
SELECT COUNT(*) FROM agent_connections;

-- 会话是否已绑 connection
SELECT COUNT(*) FROM sessions WHERE connection_id IS NOT NULL;
```

```powershell
curl.exe http://127.0.0.1:18080/healthz
```

---

## 7. 相关文档

- [DEPLOY.md](../DEPLOY.md) — 精简清单
- [系统设计](../design/SYSTEM-DESIGN.md) — connection 数据流
- [操作手册](../manual/OPERATIONS-MANUAL.md) — 日常备份与 reindex
