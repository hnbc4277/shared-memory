# Shared Memory — OpenClaw (小龙虾) Adapter

TypeScript plugin that bridges OpenClaw hooks to the Shared Memory HTTP SDK.

## Install

```bash
cd adapters/openclaw
npm install
npm run build
```

`package.json` depends on `@sharedmemory/sdk` via `file:../../sdk/typescript`.

Point OpenClaw at this package (path or link), enable plugin id `shared-memory`, and set:

```bash
SHARED_MEMORY_API_KEY=sm_...
SHARED_MEMORY_API_URL=http://127.0.0.1:18765
```

## Hooks

| Hook | Action |
|------|--------|
| `before_prompt_build` | `recall` → `{ prependContext }` |
| `message_received` | ingest user message (`agent=xiaolongxia`) |
| `agent_end` | ingest assistant reply |
| `subagent_spawned` | remember `childSessionKey` → id; later ingest uses `agent_path=xiaolongxia/<id>` |
| `subagent_ended` | drop mapping |

Standalone typecheck works without installing `openclaw` (see `src/openclaw-plugin-entry.d.ts` stub).
