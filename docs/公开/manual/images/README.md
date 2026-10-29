# 操作手册配图（真实控制台截图）

配图来自当前 `web/` 控制台（珍珠玻璃 · 总览 Bento · Ctrl+K · 智能体卡片），由 Playwright 截取并叠加编号图例（对应 [FEATURE-INDEX.md](../FEATURE-INDEX.md)）。

| PNG | 页面 |
|-----|------|
| 01-console-shell.png | 总览 Bento（`#overview`） |
| 01b-cmd-palette.png | Ctrl+K 命令面板 |
| 01c-conclusion-bar.png | 业务 Tab 连接状态条 |
| 02-login.png | API Key 登录 |
| 02b-login-email.png | 邮箱登录 |
| 02c-register.png | 注册 |
| 03-history-split.png | 历史分栏（含「当前范围」） |
| 04-agents.png | 智能体 · 卡片 +「查看全部历史会话」 |
| 04-agents-add.png | 添加连接弹窗 |
| 05-memory.png … 12-settings.png | 其余模块 |

## 重新生成

1. API：`http://127.0.0.1:18080/healthz` → `ok`
2. 仓库根：`powershell -File scripts/capture-manual-screenshots.ps1`

可选环境变量：`MANUAL_API_URL`、`MANUAL_WEB_URL`（默认 `15173`）。  
若本机 Playwright 浏览器装在 `%LOCALAPPDATA%\ms-playwright`，请设置 `PLAYWRIGHT_BROWSERS_PATH` 后再跑。

查看：浏览器打开 [图文手册.html](../图文手册.html)，或 `web` 开发服 `/docs/manual/图文手册.html`。
