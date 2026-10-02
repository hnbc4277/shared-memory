# Shared Memory — Hermes Adapter

Python MemoryProvider plugin for [Hermes Agent](https://hermes-agent.nousresearch.com/).

## Install

1. Install the Python SDK (from this repo):

```bash
pip install -e ../../sdk/python
```

Or rely on the plugin’s relative `sys.path` fallback to `sdk/python` when running from a checkout.

2. Copy this directory into Hermes:

```bash
mkdir -p ~/.hermes/plugins/memory
cp -R . ~/.hermes/plugins/memory/shared_memory
```

3. Configure Hermes (`~/.hermes/config.yaml`):

```yaml
memory:
  provider: shared_memory
```

4. Set credentials (e.g. `~/.hermes/.env`):

```bash
SHARED_MEMORY_API_KEY=sm_...
SHARED_MEMORY_API_URL=http://127.0.0.1:18765
```

Use the local daemon URL when `shared-memory-daemon` is running; otherwise point at the API.

## Behavior

| Hook | Action |
|------|--------|
| `prefetch(query, session_id)` | `Client.recall` → return `context` text |
| `sync_turn(user, assistant, session_id)` | Ingest both messages (`agent=hermes`, `external_session_id=session_id`) |
| `on_delegation(task, result)` | Ingest with `agent_path=hermes/<sub>` |
| `on_session_end` | No-op (turns already synced) |
| `get_tool_schemas` | `[]` (no extra tools) |

## Layout

```
adapters/hermes/
  plugin.yaml
  __init__.py      # SharedMemoryProvider + register(ctx)
  README.md
```
