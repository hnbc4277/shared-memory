# Cursor 最稳接法：钩子写入 + MCP 召回

适用：本机 API 在 `http://127.0.0.1:18080`，已在控制台创建 Cursor 正式连接，并抄下了专用 API 密钥。

## 推荐：一键脚本

在控制台「智能体」创建 **Cursor** 连接并勾选「专用 API 密钥」后，该行会出现 **下载配置脚本**。脚本在创建时已写入该连接的 `connection_id` 与专用密钥，下载后直接运行即可（安装目录按当前用户解析，不写死盘符/用户名）。

旧连接若没有该按钮：吊销后重新添加并勾选「专用 API 密钥」。

也可在仓库根目录对未填充的模板手工执行：

```powershell
.\scripts\setup-cursor.ps1 -ApiKey "<专用密钥>" -ConnectionId "<connection_id>"
```

程序默认装到 `%USERPROFILE%\.shared-memory\`。可用 `-InstallDir` 或环境变量 `SHARED_MEMORY_HOME` 覆盖。Cursor 配置不在默认位置时加 `-CursorConfigDir`。

下面是同一流程的手工说明。

这条接法里，**写入是强制的，召回不是**。Cursor 会丢掉钩子返回的 `additional_context`，所以不能把召回做成自动注入。

| 动作 | 谁来做 | 稳不稳 |
|------|--------|--------|
| 你发出的话、助手的回复写入共享记忆 | `smhook` | 稳。不依赖模型是否调用工具 |
| 任务开始时读出相关记忆 | 模型调用 MCP `memory_recall` | 不稳。规则只是提高概率 |

不要再让模型调用 `memory_ingest`。钩子已经在记，模型再写一遍会重复。

本机不要另开 `shared-memory-daemon`（默认 `127.0.0.1:18765`）。钩子发现守护进程在听时，会走守护进程上的密钥，专用 Key 不会生效。

## 1. 先保存这两项

- 专用 API 密钥：创建连接时只显示一次，形如 `sm_...`。页面关掉后无法再查看。
- `connection_id`：以控制台智能体页显示的为准。

下面用 `<专用密钥>`、`<connection_id>` 代替真实值。用 `<SM_HOME>` 表示共享记忆程序目录（默认 `%USERPROFILE%\.shared-memory`，或你设的 `SHARED_MEMORY_HOME` / `-InstallDir`）。

## 2. 取得两个程序

推荐用一键脚本自动下载安装。手工时也可从控制台下载，或在仓库根目录交叉编译：

```powershell
docker run --rm -v "${PWD}:/src" -w /src -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 -e GOPROXY=https://goproxy.cn,direct -e GOSUMDB=off golang:1.25-alpine sh -c "go build -o bin/smhook.exe ./cmd/smhook && go build -o bin/shared-memory-mcp.exe ./cmd/mcp"
```

把 `smhook.exe`、`shared-memory-mcp.exe` 放到 `<SM_HOME>\`。

## 3. 写入凭据文件

钩子不读 MCP 配置。密钥放在：

`<SM_HOME>\credentials.json`

```json
{
  "api_url": "http://127.0.0.1:18080",
  "api_key": "<专用密钥>",
  "connection_id": "<connection_id>"
}
```

## 4. 合并 Cursor 钩子

编辑 `%USERPROFILE%\.cursor\hooks.json`（或 Cursor 实际配置目录）。**不要整文件覆盖**，保留已有的 `sessionStart`。在 `hooks` 里追加：

```json
{
  "version": 1,
  "hooks": {
    "sessionStart": [
      {
        "command": "./hooks/superpowers-session-start.cmd"
      }
    ],
    "beforeSubmitPrompt": [
      {
        "name": "shared-memory",
        "command": "cmd /c \"set SHARED_MEMORY_HOOK_SURFACE=cursor&& <SM_HOME>\\smhook.exe\""
      }
    ],
    "afterAgentResponse": [
      {
        "name": "shared-memory",
        "command": "cmd /c \"set SHARED_MEMORY_HOOK_SURFACE=cursor&& <SM_HOME>\\smhook.exe\""
      }
    ],
    "subagentStop": [
      {
        "name": "shared-memory",
        "command": "cmd /c \"set SHARED_MEMORY_HOOK_SURFACE=cursor&& <SM_HOME>\\smhook.exe\""
      }
    ]
  }
}
```

把 `<SM_HOME>` 换成该机器上的绝对路径（一键脚本会自动写成绝对路径）。

钩子行为：

- `beforeSubmitPrompt`：把你刚发出的内容记为 user。
- `afterAgentResponse`：把助手回复记为 assistant。
- `subagentStop`：把子智能体结果记到 `cursor/<类型>`。

## 5. 配置 MCP（只用于召回）

编辑 `%USERPROFILE%\.cursor\mcp.json`，在 `mcpServers` 里增加或替换 `shared-memory`：

```json
"shared-memory": {
  "command": "<SM_HOME>\\shared-memory-mcp.exe",
  "env": {
    "SHARED_MEMORY_API_URL": "http://127.0.0.1:18080",
    "SHARED_MEMORY_API_KEY": "<专用密钥>",
    "SHARED_MEMORY_AGENT": "cursor",
    "SHARED_MEMORY_CONNECTION_ID": "<connection_id>"
  }
}
```

`command` 建议用绝对路径。保存后在 Cursor 里重载 MCP，直到出现 `memory_recall`。

## 6. 加上召回规则

新建 `%USERPROFILE%\.cursor\rules\shared-memory.mdc`：

```markdown
---
description: Shared Memory - recall at the start of non-trivial tasks
globs:
alwaysApply: true
---

# Shared Memory

每个非琐碎任务开始时：

1. 调用 `memory_recall`。query 用当前用户目标的一句话。
2. 把返回的 context 当作已知背景，不要让用户重复已经记住的偏好和决定。
3. 不要调用 `memory_ingest`。用户消息和助手回复已由钩子写入。
4. 不要在对话或工具参数里打印 API Key。
5. `memory_recall` 失败时继续任务，不要卡住。
```

这条规则只提高召回概率。模型仍可能漏调。

## 7. 怎么确认写成了

1. API 与前端已启动。
2. 完全退出 Cursor 再打开，让钩子和 MCP 重新加载。
3. 随便发一句能看出来的话。
4. 打开控制台「智能体」，该连接的最后活动应从「—」变成时间。
5. 打开「历史」，应出现刚才那条 Cursor 会话。

若历史没有新会话：看 Cursor 的 Hooks 输出里 `smhook` 是否报错。常见原因是 `credentials.json` 路径或密钥写错，或 `18765` 上有一个不该开的守护进程。

召回是否发生：控制台「密钥」页的 Recall Receipt。没有回执只说明这次模型没调用 `memory_recall`，不说明写入失败。
