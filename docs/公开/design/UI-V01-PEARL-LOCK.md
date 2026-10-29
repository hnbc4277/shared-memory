# 正式 UI 锁定：珍珠玻璃 v01

选型：**`web/design-explore/v01.html`**（西瓜同学 / scaffold 珍珠玻璃教程对标）。

## 生产代码映射

| 样稿 | 生产 |
|------|------|
| `pearl-shared.css` | `web/src/styles/pearl-shared.css` |
| `pearl-pages.css` | `web/src/styles/pearl-pages.css` |
| `pearl-runtime.js` | `web/src/pearl/neuralCanvas.ts` + `web/src/pearl/motion.ts` |
| 壳层 HTML | `web/src/App.tsx`（`stage` / `app-frame` / `rail` / `workspace`） |
| 登录页 | `web/src/pages/Login.tsx` |
| 业务页 token 兼容 | `web/src/styles/sm-bridge.css` + `web/src/styles.css` |
| 业务页组件层 | `web/src/ui.tsx`：`Section` / `StatTile` / `DataTable` / `PearlCard`（`m-surface` · `reveal` · `lift` · `panel-table`） |
| Tab 切换入场 | `web/src/pearl/motion.ts` → `pageEnter()`，由 `App.tsx` 在切换模块时调用 |
| Ctrl+K 命令面板 | `web/src/CmdPalette.tsx` + 顶栏 `cmd-trigger`（跳转各 Tab） |
| Bento 总览 | `web/src/pages/OverviewPage.tsx`（侧栏 **总览** · 真实 agents / 待确认 / 快捷入口） |
| 分段滑块 + 按压/悬抬 | `web/src/ui.tsx` `Segmented` + `web/src/pearl/interactions.ts` |

## 动效（与样稿一致的主链）

1. 登录页入场（GSAP stagger）
2. 提交成功 → 按钮收束为 **success-orb** 描边对勾
3. **NeuralNet burst**（Canvas）
4. 控制台 **shell entrance** + 侧栏 **nav-thumb** 滑动

尊重 `prefers-reduced-motion`：动效模块内短路，CSS 仍有 backstop。

## 规格与规则

- 产品定义：`.workbench/spec.md`（`visual` 字段）
- Cursor 规则：`.cursor/rules/scaffold-admin-ui-style.mdc`（需与 v01 同步阅读）
- 样稿迭代：仍可在 `web/design-explore/` 改 v01–v10，确认后再回迁 `web/src/styles/`

## 本地验证

```bash
cd web
npm run dev
```

浏览器登录一次，确认对勾 + 迸发 + 主内容可见。

## 截图与生产差异（Ctrl+K / Bento / p07）

选型图与当前 `web/` 的对照说明见 **[UI-SCREENSHOT-GUIDE.md](./UI-SCREENSHOT-GUIDE.md)**（含 `docs/公开/design/screenshots/` 各 PNG 标注）。
