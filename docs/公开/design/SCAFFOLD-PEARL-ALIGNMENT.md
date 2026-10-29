# 珍珠玻璃对齐说明（视频 / scaffold ↔ 共享记忆控制台）

参考：[西瓜同学🍉 抖音样片](https://v.douyin.com/io_6I0Z_g94/)、本地 clone `scaffold-master`（`DESIGN.md` + `scaffold-vue/src/styles`）。

## 先前 v01–v10 为何「不像视频」

| 维度 | 旧样稿（v01–v10） | scaffold / 视频（珍珠玻璃） |
| --- | --- | --- |
| 侧栏 | 多为 **近黑 / Apple 深蓝** 实色侧栏 | **浅色导航轨** `m-rail`（`--rail-bg` 高不透明浅灰白 + blur） |
| 主色 | `#0066cc` 等通用蓝 | **`#236de9`** 主操作 + **`#82dd2d` / `#58a021`** 青柠状态 |
| 外壳 | 全屏铺平，无「设备框」 | **`stage` 灰底 + `app-frame` 10px 深色描边 + 38px 圆角** |
| 画布 | 单色内容区 | **`--canvas` + 两团静态环境光**（青柠 / 蓝，极淡） |
| 选中导航 | 色条或整块高亮底色 | **一块 `m-pill` 玻璃 thumb 在项之间滑动**（GSAP `slideThumb`） |
| 登录 | 简单双栏表单 | **左全幅深色画布**（NeuralNet + 网格 + 时钟盘 + 健康铭牌）+ **右登录卡 ≤4° 指针倾角** + 入场编排 |
| 动效 | CSS `transition` 为主 | **GSAP**：按压缩放 0.955、页面 scale 进场、`expo.out` / `power4.out` 等 |
| 材质 | 普通 border + shadow | **分层**：`m-rail` / `m-pill` / `m-float` / `m-surface` / `m-groove`，白边 + inset 高光 |

结论：**不是「再换一套 hex」**，而是 **舞台 + 浅轨 + 玻璃 thumb + 青柠语义 + GSAP 手感** 一整套；旧样稿只借了「后台」轮廓，没有借材质与动效语汇。

## 新高保真静态样稿

| 入口 | 内容 |
| --- | --- |
| [web/design-explore/index.html](../../web/design-explore/index.html) | **珍珠玻璃 10 款** `p01`–`p10`（每款含登录 + 控制台，见 `generate-pearl.mjs`） |
| [web/design-explore/scaffold-pearl-login.html](../../web/design-explore/scaffold-pearl-login.html) | 首版单页登录（对照） |
| [web/design-explore/scaffold-pearl-console.html](../../web/design-explore/scaffold-pearl-console.html) | 首版单页壳层（对照） |

本地查看：浏览器打开 `web/design-explore/index.html` 或任意 `p0N.html`（需联网加载 GSAP CDN）。

## 选定后落地到 `web/` 的顺序

1. 将 `pearl-shared.css` 中的 `:root` 令牌合并进 `web/src/styles.css`（或拆 `tokens.css` 再在 Vite 引入）。
2. 重构壳层：`App.tsx` → `stage` / `app-frame` / `rail m-rail` / `nav-thumb m-pill` / `workspace`。
3. 引入 **GSAP**（与 scaffold 同：`installPressFeedback`、`slideThumb`、`playShellEntrance`、`pageEnter`）。
4. `Login.tsx` 对齐 `Login.vue` 布局与 intro 时间轴（业务字段保持 API Key / 邮箱等你方现有逻辑）。
5. 更新 `.cursor/rules/scaffold-admin-ui-style.mdc`：**以珍珠玻璃为主叙述**，深色侧栏仅作历史对照。
6. UI 稳定后重跑 `scripts/capture-manual-screenshots.ps1` 更新手册配图。

## 与业务的边界

- 样稿 **不改 API、路由、Tab 结构**；智能体页仅作视觉占位（分组、在线点、表格密度）。
- 图谱舞台、Bento 工作台等 scaffold **可选增强**；共享记忆控制台 MVP 优先：**壳层 + 登录 + 列表页材质一致**。
