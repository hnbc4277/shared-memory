# HTTP API 索引（自托管）

Base URL 默认 `http://127.0.0.1:18080`。鉴权：

- **智能体 / MCP / CLI**：`Authorization: Bearer sm_...`（API Key）
- **Web 控制台（Cookie 模式）**：登录后 HttpOnly `sm_session`（`wss_...`），与 Bearer 二选一

公开：`GET /healthz`，`POST /v1/auth/register|login`（register 同时 `Set-Cookie` + 返回 `api_key`），设备码流程。

限流（需 Redis）：账号热路径 `RATE_LIMIT_PER_MIN`；登录 IP `AUTH_RATE_LIMIT_PER_MIN`；登录邮箱 `AUTH_EMAIL_RATE_LIMIT_PER_MIN`（见 `.env.example`）。

## 账号与密钥

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v1/auth/register` | 注册（返回一次性 API Key） |
| POST | `/v1/auth/login` | 邮箱密码 → Cookie + `wss_` token |
| POST | `/v1/auth/logout` | 吊销 web session |
| GET | `/v1/me` | 当前 account_id |
| GET/POST/DELETE | `/v1/auth/api-keys` | 列表（含专用 connection 关联）/ 创建 / 吊销 |

## 证据与召回

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v1/ingest` | 写入消息（限流） |
| POST | `/v1/recall` | 召回（限流；可选 `connection_id`） |
| GET | `/v1/recall/receipts` | Receipt 列表 |
| GET | `/v1/sessions` | 会话列表（`connection_id` 筛选） |

## Embedding

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/embedding-settings` | 账号覆盖（不含 API Key 明文）+ 部署摘要 + `source` + `reindex_required` + `max_dim` |
| PUT | `/v1/embedding-settings` | 保存前探测维度；超过 `EMBEDDER_MAX_DIM` 返回 400；配置变化则 `reindex_required` |
| DELETE | `/v1/embedding-settings` | 关闭账号覆盖；与已重建指纹不一致时保持 `reindex_required` |
| POST | `/v1/embedding-settings/probe` | 只探测维度，不落库 |
| POST | `/v1/embedding-settings/reindex` | 按账号从 Postgres 重建向量；成功后清除 `reindex_required` |

## 智能体连接

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/agents` | 分组实例 |
| GET | `/v1/agents/stream` | SSE 在线状态（含 `process_online` / `activity_fresh` / `online`） |
| POST | `/v1/agents/connections` | 创建（可选专用 Key、`claim_history`） |
| POST | `/v1/agents/connections/heartbeat` | 连接进程保活（更新 `last_heartbeat_at`；见下） |
| POST | `/v1/agents/connections/{id}/claim-history` | 认领历史会话 |
| PATCH/DELETE | `/v1/agents/connections/{id}` | 备注 / 吊销 |

### `POST /v1/agents/connections/heartbeat`

连接级进程在线心跳（setup 保活、MCP、adapter 等）。鉴权：Bearer API Key 或 Web Cookie。

- **connection_id**：专用 Key 可省略（默认该 Key 绑定的连接）；账号 Key 须在 JSON body 或 query `connection_id` 中指定。
- **Body（可选 JSON）**：`client`（如 `setup` · `mcp` · `adapter` · `daemon`）、`version`；仅审计，不落库分列。
- **成功：** `204 No Content`（更新 `last_heartbeat_at`；同连接 **5s** 内重复请求仍返回 204 但不写库，降低写放大）。
- **错误：** 缺少/无效 `connection_id` → `400`；Key 与连接不匹配 → `403`；未知或已吊销 → `404`。

列表 / SSE 在 90s 窗口内反映 `process_online=true`。详见 `[internal-notes]

## 记忆、实体、KB、凭证、成稿

- 记忆：`GET/PUT/DELETE /v1/memories`，`GET /v1/profile`
- 实体：`GET/POST/PATCH/DELETE /v1/entities`
- KB：`POST/GET/DELETE /v1/kb/documents`（上传限流）
- 凭证：`/v1/vault/*`，`GET/POST /v1/vault/wrap-status|wrap-setup`
- 成稿：`/v1/artifacts/*`

## WebSocket

`GET /v1/ws` — JSON 消息 `ping|recall|ingest`；鉴权同 HTTP（Bearer、`wss_`、Cookie、query `access_token`、首包 `auth`）。

完整行为见 `[internal-notes] 与各子规格。机器可读摘要：`docs/公开/openapi.yaml`。
