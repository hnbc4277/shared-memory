/** Connection presence ticker — POST /v1/agents/connections/heartbeat while the plugin is loaded. */

export const DEFAULT_HEARTBEAT_INTERVAL_MS = 30_000;

export function heartbeatIntervalMs(): number {
  const raw = (process.env.SHARED_MEMORY_HEARTBEAT_INTERVAL ?? "").trim();
  if (raw) {
    const n = Number(raw);
    if (Number.isFinite(n) && n > 0) {
      return Math.round(n * 1000);
    }
  }
  return DEFAULT_HEARTBEAT_INTERVAL_MS;
}

export type HeartbeatOpts = {
  baseUrl: string;
  apiKey: string;
  fetchFn?: typeof fetch;
  intervalMs?: number;
};

/** Starts an immediate ping then repeats. Failures are swallowed. Returns a stop function. */
export function startConnectionHeartbeat(opts: HeartbeatOpts): () => void {
  const base = (opts.baseUrl || "").replace(/\/$/, "");
  const key = opts.apiKey || "";
  if (!base || !key) {
    return () => {};
  }
  const fetchFn = opts.fetchFn ?? fetch;
  const interval = opts.intervalMs ?? heartbeatIntervalMs();

  const ping = () => {
    const cid = (process.env.SHARED_MEMORY_CONNECTION_ID ?? "").trim();
    const body: Record<string, string> = { client: "adapter" };
    if (cid) body.connection_id = cid;
    fetchFn(`${base}/v1/agents/connections/heartbeat`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${key}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    }).catch(() => {
      // Network failures must not break the host.
    });
  };

  ping();
  const id = setInterval(ping, interval);
  return () => clearInterval(id);
}
