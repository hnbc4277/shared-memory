# 自托管验收清单

## tool_calls（History）

1. 通过 ingest 写入带 `tool_calls` 的消息（见 adapter 文档或 smoke 的 `New-Message` 扩展）。
2. 打开 **历史 → 会话详情**：应看到工具 Chip + 起止时间；若有 `output`，在「输出（截断）」中展开。
3. 后端 ingest 将超过 4000 runes 的输出截断存库（`output_truncated`）。

## 生产 P0（脚本）

```powershell
# 构建 + 可选 reindex + smoke + tag
powershell -File scripts/ops/selfhosted-release.ps1 -Reindex -Smoke -Tag v0.1.0 -Api https://your-api
```

```bash
SHARED_MEMORY_ENV=.env bash scripts/ops/reindex.sh
API=https://your-api bash scripts/e2e-smoke.sh
psql "$POSTGRES_DSN" -f scripts/ops/verify-migrations.sql
```

备份示例：`bash scripts/ops/backup-postgres.example.sh`（配合 cron）。
