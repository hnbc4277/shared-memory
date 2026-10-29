---
name: shared-memory
description: 跨智能体共享记忆。在任务开始前召回项目偏好与历史决策；在用户明确偏好、重要结论或跨会话事实出现后写入。当用户提到「记得」「上次」「偏好」「项目约定」、需要跨 Cursor/豆包等客户端的上下文，或你要避免让用户重复已说过的信息时使用本技能。
---

# Shared Memory（豆包 · MCP + Skill）

豆包工作没有静默 hook。本技能指导你**主动**调用已配置的 Shared Memory MCP 工具；连接器负责读写，本技能负责何时调用。

## 前置

- 已在「插件·技能·伙伴 / 连接器」中配置 Shared Memory MCP（见同目录 `mcp.connector.example.json` 或 `smctl install doubao` 生成的 `mcp.connector.json`）。
- 上报名固定为 `agent=doubao`（由 MCP 环境变量 `SHARED_MEMORY_AGENT` 默认注入；调用时勿改成其它产品名）。

## 召回（每个非琐碎任务开始时）

1. 先调用 `memory_recall_status`（若可知会话 id，带上 `external_session_id`，建议形如 `doubao/<task-or-chat-id>`）。
2. 若返回 `need_recall: true`，必须先调用 `memory_recall` 再继续：
   - `query`：当前用户目标的一句话摘要
   - 已知项目时带 `project_id`
   - 尽量带 `external_session_id`，便于控制台 Recall Receipt 关联
3. 把返回的 `context`（画像、项目摘要、相关事实、知识库片段）当作已知背景；**不要**让用户重复已经记住的偏好、决策与项目事实。
4. 若 `memory_recall` 失败或超时，正常继续任务，不要卡死。

## 写入

- 重要结论、用户明确偏好、项目决策：回复后可通过 `memory_ingest` 写入（`role=assistant` 或 `user`）。
- 填齐 `skills` / `plugins` / `tools`（没有则传 `[]`）。
- 需要把规范/笔记进知识库时用 `kb_add`；**同意保存成稿不等于进知识库**。

## 连接归属

- 多实例时在 Web「智能体」添加连接，把 `connection_id` 写入 MCP 环境变量 `SHARED_MEMORY_CONNECTION_ID`，或使用该连接的**专用 API 密钥**（可省略显式 `connection_id`）。

## 安全

- 绝不要在对话或工具参数里打印 API Key、口令或 vault 明文。
