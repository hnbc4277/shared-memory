# 操作手册配图（真实控制台截图）

配图来自 **当前 `web/` 控制台的真实页面**（珍珠玻璃 v01 · 总览 Bento · Ctrl+K），由 Playwright 自动截取，并在图上叠加 **编号 + 底部图例**（对应 [FEATURE-INDEX.md](../FEATURE-INDEX.md) 中的功能编号）。

| PNG | 页面 |
|-----|------|
| 01-console-shell.png | 登录后 **总览** Bento（`#overview`，默认首页） |
| 01b-cmd-palette.png | **Ctrl+K** 命令面板 |
| 01c-conclusion-bar.png | 业务 Tab **连接状态条**（示例：历史页） |
| 02-login.png | API 密钥登录（分段滑块） |
| 02b-login-email.png | 邮箱登录 |
| 02c-register.png | 注册表单 |
| 02c-register-issued.png | 注册成功 · 一次性密钥 |
| 03-history-split.png | 历史（分栏/列表分段） |
| 04-agents.png | 智能体 · 分组列表与行操作 |
| 04-agents-add.png | 智能体 · 「添加连接」面板 |
| 05-memory.png | 记忆 |
| 06-vault.png | 凭证库 |
| 07-artifacts.png | 成稿库 |
| 08-kb.png | 知识库 |
| 09-projects.png | 项目 |
| 10-keys.png | 密钥与回执 |
| 11-device.png | 授权设备 |
| 12-settings.png | 设置 |

## 重新生成（界面改版后必跑）

1. 确保 API 正常：`http://127.0.0.1:18080/healthz` → `ok`（`docker compose --profile app up -d --build`）。
2. 在项目根目录执行：

```powershell
powershell -File scripts/capture-manual-screenshots.ps1
```

脚本会：`web` 构建 → 启动 `vite preview` → 注册演示账号并 ingest 样例数据 → 逐页截图 → 同步到 `web/public/docs/manual/images/`。

环境变量（可选）：

- `MANUAL_API_URL` — 默认 `http://127.0.0.1:18080`
- `MANUAL_WEB_URL` — 默认 `http://127.0.0.1:15173`

标注定义在 `scripts/manual-screenshots/scenes.mjs`；找不到控件时会在终端输出 `[callout] 未找到`。

## 查看

- 浏览器打开 [图文手册.html](../图文手册.html)
- 或 `cd web && npm run dev` → `http://127.0.0.1:15173/docs/manual/图文手册.html`

旧版 **SVG 线框** 仍保留在同目录 `*.svg`，仅作备份；手册正文以 **PNG 截图** 为准。
