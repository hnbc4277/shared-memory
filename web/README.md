# 共享记忆 · Web 控制台

Vite + React 18 + TypeScript。只调用本项目 API（默认 `http://127.0.0.1:18080`），不依赖任何本机全局服务。

## 运行

```powershell
# 1. 启动项目自带的隔离环境与 API（仓库根目录）
docker compose --profile app up -d --build   # 或：make up && make api

# 2. 启动控制台
cd web
npm install --registry=https://registry.npmmirror.com
npm run dev        # http://127.0.0.1:15173
```

环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `VITE_API_BASE` | `http://127.0.0.1:18080` | API 地址；API 侧需在 `CORS_ORIGINS` 中放行控制台来源 |
| `VITE_REPO_DOCS_BASE` | （空） | 可选：GitHub blob 根路径，智能体 Tab 文档链指向仓库；未设时使用 `web/public/` 下静态副本 |

发布前同步静态文档：`npm run sync-docs`（等价于仓库根目录 `scripts/sync-web-docs.ps1`，从 `[internal-notes] 等拷贝到 `web/public`），再 `npm run build`。CI 会执行根目录 `node scripts/check-web-docs-sync.mjs` 校验 `web/public` 未过期。仓库文档划分见 `docs/README.md`。

其他命令：`npm test`（vitest 单测）、`npm run build`（类型检查 + 产物输出到 `dist/`）。

## 登录

- **邮箱登录（推荐）**：邮箱 + 密码 → API 设置 HttpOnly `sm_session`（`wss_...`），控制台以 Cookie 模式调用 API（`credentials: include`）。**API 的 `CORS_ORIGINS` 必须包含控制台来源**，且不能为 `*`。
- **注册账号**：仍会签发一把 **API Key**（只显示一次），供 MCP/适配器使用；浏览器会话不依赖该 Key。
- **仅 API Key**：粘贴 `sm_...`，保存在 `localStorage`（`sm.apiKey`），适合无 Cookie 场景。

点击「退出登录」会吊销 web session 并清除本地 Key。

## 手动验收清单

准备：用 `scripts/e2e-smoke.ps1` 或任意智能体写入几条带 `agent_path`（含子智能体，如 `xiaolongxia/researcher`）、skills、plugins 的消息，其中包含一条含账号口令的消息和一条 ≥800 字的助手回复。

1. **登录**：注册新账号后进入「历史」；退出后用另一账号登录，看不到前一账号的任何数据（账号隔离）。
2. **项目切换**：侧栏切换项目，历史 / 记忆 / 成稿列表随之过滤；「全部 / 全局」显示所有项目。
3. **历史与溯源**：会话列表显示来源智能体与时间；详情中每条消息展示 `agent_path`、skills、plugins、tools、model 与时间；按 `xiaolongxia` 过滤时包含其子智能体会话；消息中的口令显示为 `[REDACTED]`。
4. **记忆**：召回测试输入相关问题能看到会话摘要 / 事实 / 项目近况，无关问题不返回无关条目；「注入给智能体的上下文」可展开；对记忆执行「更正」「失效」后再次召回结果随之变化；用户画像来自偏好记忆。
5. **凭证库**：待确认中出现检测到的口令，「丢弃」后不再出现；「加密保存」后进入凭证库，口令默认隐藏；「查看」需再次输入账号密码，错误密码被拒绝，口令 30 秒后自动隐藏；「设置 → 审计日志」能看到查看与同意记录。
6. **成稿库**：长回复出现在待确认中，保存后可查看、编辑（生成新版本）、导出 Markdown；手动新建含口令的成稿会提示风险，需确认后才保存。
7. **项目**：新建项目；删除项目需二次确认，删除后其会话、记忆、成稿与检索索引一并消失。
8. **设置**：关闭「保存聊天原文」后新写入的消息仅保留结构化溯源，会话详情提示「原文未保存」；关闭「自动注入记忆」后召回测试提示 `inject_disabled`；「原文保留时长」设为 N 天后，worker 每小时删除超期会话的原文、消息与附件，派生记忆保留；危险区删除账号经确认后清空全部数据并退出。
9. **密钥**：新建 Key 仅展示一次，复制 MCP 配置片段可直接用于 Cursor / Codex；吊销后该 Key 请求返回 401。
