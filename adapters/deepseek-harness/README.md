# Shared Memory — DeepSeek Harness Adapter

Thin config package: reuse **Codex-dialect hooks** (`smhook`) plus **MCP** (`shared-memory-mcp`) inside DeepSeek Harness via Cordis plugins.

## Files

| File | Purpose |
|------|---------|
| `cordis.yml.example` | Registers `@deepseek-ai/dsh-mcp-client` → `shared-memory-mcp` and `@deepseek-ai/dsh-hooks-codex` → `hooks.json` |
| `hooks.json.example` | Codex-shaped hook commands invoking `smhook` with `SHARED_MEMORY_HOOK_SURFACE=codex` |

## Setup

1. Build/install binaries so `shared-memory-mcp` and `smhook` are on `PATH` (or edit absolute paths in the examples).

2. Copy examples into your harness project:

```bash
cp cordis.yml.example /path/to/project/cordis.yml   # or merge the plugins: block
cp hooks.json.example /path/to/project/hooks.json
```

3. Environment:

```bash
export SHARED_MEMORY_API_KEY=sm_...
export SHARED_MEMORY_API_URL=http://127.0.0.1:18765
```

4. Start DeepSeek Harness with that `cordis.yml`.

## Agent naming

Spec agent id is `deepseek-harness`. The example hooks set `SHARED_MEMORY_AGENT=deepseek-harness` for when `smhook` honors that override. Until then, Codex-surface hooks still tag ingest as `agent=codex` (same protocol); MCP tools are agent-agnostic.

## Behavior

| Path | Role |
|------|------|
| `dsh-hooks-codex` + `smhook` | Auto ingest + Codex-style `additionalContext` recall |
| `dsh-mcp-client` + `shared-memory-mcp` | Model-callable `memory_recall` / related tools |
