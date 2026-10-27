# Shared Memory — adapter keepalive scripts

Cross-platform loop used by `smctl install` for Hermes / OpenClaw / DeepSeek Harness.

```text
POST /v1/agents/connections/heartbeat
Authorization: Bearer $SHARED_MEMORY_API_KEY
{"client":"setup","connection_id":"$SHARED_MEMORY_CONNECTION_ID"}
进程运行期间每 30s 一次
```

Adapter/plugin processes use `client: adapter` instead; these scripts are the **install-side** keepalive (`client: setup`) when the product itself is not running.

```bash
export SHARED_MEMORY_API_URL=http://127.0.0.1:18765
export SHARED_MEMORY_API_KEY=sm_...
export SHARED_MEMORY_CONNECTION_ID=...   # optional with a dedicated key
chmod +x sm-connection-heartbeat.sh
nohup ./sm-connection-heartbeat.sh >/tmp/sm-connection-heartbeat.log 2>&1 &
```

```powershell
$env:SHARED_MEMORY_API_URL = "http://127.0.0.1:18765"
$env:SHARED_MEMORY_API_KEY = "sm_..."
$env:SHARED_MEMORY_CONNECTION_ID = "..."
powershell -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File .\sm-connection-heartbeat.ps1
```

Task Scheduler / systemd --user is optional and not registered automatically.
