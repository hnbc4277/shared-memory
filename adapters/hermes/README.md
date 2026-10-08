# Shared Memory — Hermes Adapter

Python MemoryProvider plugin for [Hermes Agent](https://hermes-agent.nousresearch.com/).

## 一键安装（推荐）

在仓库根目录（或已放置 `adapters/hermes` 的环境）执行：

```bash
smctl login          # 若尚未登录
smctl install hermes
```

会把插件复制到 `~/.hermes/plugins/memory/shared_memory/`，并生成 `INSTALL.txt` 与
`~/.hermes/shared-memory-config.snippet.yaml`。

然后在 `~/.hermes/config.yaml` 设置：

```yaml
memory:
  provider: shared_memory
```

凭证写入 `~/.hermes/.env`（`smctl install` 的 INSTALL.txt 含当前凭据模板）：

```bash
SHARED_MEMORY_API_KEY=sm_...
SHARED_MEMORY_API_URL=http://127.0.0.1:18765
```

## 手动安装

1. `pip install -e ../../sdk/python`（或依赖插件内相对路径回退到 `sdk/python`）
2. `cp -R adapters/hermes ~/.hermes/plugins/memory/shared_memory`
3. 同上配置 `memory.provider` 与环境变量

## 行为（A+B）

| Hook | Action |
|------|--------|
| `prefetch` | **自动注入**：`recall` → context |
| `sync_turn` | 上报 user/assistant；尽力填写 skills/plugins/tools/model |
| `on_delegation` | `agent_path=hermes/<sub>` |
| 失败时 | 控制台查看 Recall Receipt；可 MCP/`memory_recall` 补召回 |

## Layout

```
adapters/hermes/
  plugin.yaml
  __init__.py
  README.md
```
