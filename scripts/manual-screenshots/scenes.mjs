/** @typedef {import('./callouts.mjs').Callout} Callout */

const WEB = (process.env.MANUAL_WEB_URL ?? "http://127.0.0.1:15173").replace(/\/+$/, "");

/**
 * @typedef Scene
 * @property {string} file
 * @property {boolean} [auth]
 * @property {string} [hash]
 * @property {(page: import('playwright').Page, seed: object) => Promise<void>} [setup]
 * @property {Callout[]} callouts
 * @property {boolean} [fullPage]
 */

/** @type {Scene[]} */
export const SCENES = [
  {
    file: "02-login.png",
    callouts: [
      { id: "L-01", label: "切换到 API Key 登录", name: "API Key" },
      { id: "L-02", label: "粘贴 sm_ 开头密钥", selector: ".login-card input[type='password']" },
      { id: "L-03", label: "校验通过后进入控制台", name: "进入控制台" },
      { id: "L-09", label: "画布区显示 API 地址（应对 healthz 可达）", selector: ".login-plate.is-health" },
    ],
  },
  {
    file: "02b-login-email.png",
    setup: async (page) => {
      await page.getByRole("button", { name: "邮箱登录" }).click();
    },
    callouts: [
      { id: "L-03", label: "切换到邮箱登录", name: "邮箱登录" },
      { id: "L-04", label: "填写邮箱与密码后登录", name: "登录" },
    ],
  },
  {
    file: "02c-register.png",
    setup: async (page) => {
      await page.locator(".login-mode-seg button", { hasText: /^注册$/ }).click();
    },
    callouts: [
      { id: "L-05", label: "切换到注册", selector: ".login-mode-seg button.is-active" },
      { id: "L-06", label: "邮箱 + 密码（≥8 位）提交注册", selector: ".login-form button.btn-primary" },
    ],
  },
  // 注册成功态需真实提交；演示账号流程不强制截此帧（手册以线框说明）
  {
    file: "01-console-shell.png",
    auth: true,
    hash: "overview",
    callouts: [
      { id: "S-00", label: "侧栏 · 总览（登录默认）", name: "总览" },
      { id: "S-01", label: "侧栏 · 历史", name: "历史" },
      { id: "S-03", label: "侧栏 · 智能体", name: "智能体" },
      { id: "S-19", label: "顶栏 · 命令面板入口", selector: ".cmd-trigger" },
      { id: "S-14", label: "顶栏 · 当前项目上下文", selector: ".topbar-project select" },
      { id: "S-11", label: "退出登录", name: "退出" },
      { id: "O-01", label: "总览 · 连接概览 Hero", selector: ".bento-hero" },
      { id: "O-02", label: "总览 · 跳转智能体", name: "智能体连接" },
      { id: "O-06", label: "总览 · 常用入口", selector: ".module-tile, .quick-link, .bento-tile" },
    ],
  },
  {
    file: "01b-cmd-palette.png",
    auth: true,
    hash: "overview",
    setup: async (page) => {
      await page.locator(".cmd-trigger").click();
      await page.waitForSelector(".cmd-overlay.is-open", { timeout: 8000 });
    },
    callouts: [
      { id: "S-19", label: "Ctrl+K 打开命令面板", selector: ".cmd-panel input" },
      { id: "S-20", label: "选择模块后 Enter 跳转", selector: ".cmd-item" },
      { id: "S-03", label: "列表示例 · 智能体", name: "智能体" },
    ],
  },
  {
    file: "01c-conclusion-bar.png",
    auth: true,
    hash: "history",
    callouts: [
      { id: "S-16", label: "业务 Tab · 连接状态条", selector: ".conclusion-bar" },
      { id: "S-17", label: "跳转智能体", selector: ".conclusion-action, .conclusion-bar" },
    ],
  },
  {
    file: "03-history-split.png",
    auth: true,
    hash: "history",
    setup: async (page) => {
      await page.waitForSelector(".split, form.row", { timeout: 15000 }).catch(() => {});
      const first = page.locator("ul.list li.item").first();
      if ((await first.count()) > 0) await first.click().catch(() => {});
    },
    callouts: [
      { id: "H-03", label: "筛选 · 智能体路径", selector: "form.row input.wide, form.row.wrap input" },
      { id: "H-05", label: "应用筛选", name: "过滤" },
      { id: "H-16", label: "当前范围提示", selector: ".history-scope, p.muted.small" },
      { id: "H-06", label: "点击左侧会话行查看详情", selector: "ul.list li.item" },
    ],
  },
  {
    file: "04-agents.png",
    auth: true,
    hash: "agents",
    setup: async (page) => {
      await page.waitForLoadState("networkidle").catch(() => {});
      await page.waitForSelector(".stat-row, .empty, .error-line", { timeout: 25000 });
      await page.waitForSelector("button.agent-group-head", { timeout: 45000 }).catch(() => {});
    },
    callouts: [
      { id: "A-01", label: "在线/离线统计", selector: ".stat-row" },
      { id: "A-02", label: "添加正式连接", name: "添加连接" },
      { id: "A-15", label: "账号下全部历史", name: "查看全部历史会话" },
      { id: "A-08", label: "按产品折叠/展开分组", selector: "button.agent-group-head" },
      { id: "A-09", label: "查看该产品历史", name: "查看历史" },
      { id: "A-10", label: "编辑连接备注", name: "编辑" },
      { id: "A-11", label: "仅本连接历史", name: "查看历史会话" },
      { id: "A-14", label: "展开最近召回回执", name: "查看回执" },
    ],
  },
  {
    file: "04-agents-add.png",
    auth: true,
    hash: "agents",
    setup: async (page) => {
      await page.waitForLoadState("networkidle").catch(() => {});
      await page.waitForSelector("button.agent-group-head", { timeout: 45000 }).catch(() => {});
      await page.getByRole("button", { name: "添加连接" }).click();
      await page.waitForSelector(".modal, [role='dialog']", { timeout: 10000 }).catch(() => {});
    },
    callouts: [
      { id: "A-03", label: "选择产品（Cursor / 豆包等）", selector: ".modal select, [role='dialog'] select" },
      { id: "A-04", label: "连接备注", selector: ".modal input[type='text'], [role='dialog'] input[type='text']" },
      { id: "A-05", label: "可选：签发专用 API 密钥", selector: ".modal .toggle-inline, [role='dialog'] .toggle-inline" },
      { id: "A-07", label: "提交创建", name: "创建" },
      { id: "A-02", label: "关闭面板", name: "取消" },
    ],
  },
  {
    file: "05-memory.png",
    auth: true,
    hash: "memory",
    callouts: [
      { id: "M-02", label: "召回测试问题", selector: "input[placeholder*='向量库']" },
      { id: "M-04", label: "执行召回", name: "召回" },
      { id: "M-12", label: "记忆类型筛选", selector: ".filter-panel select" },
      { id: "M-09", label: "添加实体别名", name: "添加" },
      { id: "M-13", label: "更正单条记忆", name: "更正" },
    ],
  },
  {
    file: "06-vault.png",
    auth: true,
    hash: "vault",
    callouts: [
      { id: "V-01", label: "待确认候选统计", selector: ".stat-row" },
      { id: "V-02", label: "候选 · 加密保存", name: "加密保存" },
      { id: "V-03", label: "候选 · 不保存", name: "不保存" },
      { id: "V-04", label: "手动添加凭证", name: "手动添加" },
      { id: "V-05", label: "已保存 · 查看口令", name: "查看" },
    ],
  },
  {
    file: "07-artifacts.png",
    auth: true,
    hash: "artifacts",
    callouts: [
      { id: "V-02", label: "候选 · 保存到成稿库", name: "保存" },
      { id: "V-03", label: "候选 · 不保存", name: "不保存" },
      { id: "AR-01", label: "新建成稿", name: "新建" },
    ],
  },
  {
    file: "08-kb.png",
    auth: true,
    hash: "kb",
    callouts: [
      { id: "KB-01", label: "可选标题", selector: "input[placeholder='标题（可选）']" },
      { id: "KB-02", label: "选择文件即上传（md/pdf/docx 等）", selector: "input[type='file']" },
    ],
  },
  {
    file: "09-projects.png",
    auth: true,
    hash: "projects",
    callouts: [
      { id: "P-01", label: "新建项目名称", selector: "input[placeholder='新项目名称']" },
      { id: "P-02", label: "创建项目", name: "创建" },
      { id: "P-03", label: "表格中的 project_id（配置到 ingest）", selector: "table.table code" },
    ],
  },
  {
    file: "10-keys.png",
    auth: true,
    hash: "keys",
    callouts: [
      { id: "K-01", label: "生成新 API 密钥", name: "生成" },
      { id: "K-02", label: "吊销密钥", name: "吊销" },
      { id: "K-03", label: "召回回执列表", selector: "#sm-receipts, section.card" },
      { id: "K-04", label: "跳转授权新设备", name: "授权新设备" },
    ],
  },
  {
    file: "11-device.png",
    auth: true,
    hash: "device",
    callouts: [
      { id: "D-01", label: "输入 smctl 用户码", selector: "input" },
      { id: "D-02", label: "确认授权设备", name: "授权此设备" },
    ],
  },
  {
    file: "12-settings.png",
    auth: true,
    hash: "settings",
    callouts: [
      { id: "SET-01", label: "会话/原文保留策略", selector: ".toggle" },
      { id: "SET-02", label: "隔离区条目 · 重放入队（有不合格上报时显示）", selector: "section.card h2" },
      { id: "SET-03", label: "永久删除账号", name: "永久删除账号及全部数据" },
    ],
  },
];
