# v01 / p07 选型截图 · 快速索引

**详细图文说明（推荐阅读）**：[../UI-SCREENSHOT-GUIDE.md](../UI-SCREENSHOT-GUIDE.md)

---

## 截图文件一览

| 文件 | 样稿来源 | 一句话 |
|------|----------|--------|
| [v01-console-bento.png](./v01-console-bento.png) | `web/design-explore/v01.html` | **Bento 工作台**：大 KPI 卡片 + 常用入口，顶栏带 **Ctrl+K** 按钮 |
| [v01-cmd-palette.png](./v01-cmd-palette.png) | v01（常含登录段 + 面板打开） | **命令面板**：输入框 + 菜单项列表，**Ctrl+K** 唤起 |
| [v02-segmented-and-cmd.png](./v02-segmented-and-cmd.png) | `v02.html` | **列表页 + Ctrl+K**（无 Bento）；与生产业务页结构更接近 |
| [p07-shared-memory-console.png](./p07-shared-memory-console.png) | `p07.html` | **Shared Memory 菜单** + 智能体 **四格 Stat + 连接表**；顶栏仅「搜索…」，**无 Ctrl+K** |
| [p07-agents-like-prod.png](./p07-agents-like-prod.png) | p07 控制台段裁切 | 便于和手册 `docs/公开/manual/images/04-agents.png` 对照 |

---

## 生产控制台更像谁？

| 维度 | 更像 |
|------|------|
| 材质、登录、侧栏 thumb、表格/Stat 组件 | v01 **风格** + **p07 信息架构** |
| 首页布局 | **p07 各 Tab**（StatRow + 表），**不是** v01 Bento |
| 顶栏 | 自成一套（项目/退出），**既不像** v01 Ctrl+K **也不像** p07 假搜索框 |

---

## 本地打开样稿

于 `web/design-explore` 执行 `python -m http.server 8765`，访问 `v01.html` / `v02.html` / `p07.html`（可加 `#console` 跳到控制台段）。
