import { API_BASE } from "./api/client";

const KEY_STORAGE = "sm.apiKey";
const AUTH_MODE = "sm.authMode";

export type AgentStreamPayload = { groups: unknown[] };

/** Reads GET /v1/agents/stream with Bearer auth (fetch; EventSource cannot set headers). */
export function subscribeAgentsStream(
  onData: (payload: AgentStreamPayload) => void,
  onError?: (err: Error) => void,
): () => void {
  const cookieMode = localStorage.getItem(AUTH_MODE) === "cookie";
  const key = localStorage.getItem(KEY_STORAGE);
  if (!cookieMode && !key) {
    return () => {};
  }
  const ac = new AbortController();
  let buffer = "";

  void (async () => {
    try {
      const headers: Record<string, string> = {};
      if (key) headers.Authorization = `Bearer ${key}`;
      const resp = await fetch(`${API_BASE.replace(/\/+$/, "")}/v1/agents/stream`, {
        headers,
        credentials: cookieMode ? "include" : undefined,
        signal: ac.signal,
      });
      if (!resp.ok || !resp.body) {
        throw new Error(
          resp.status === 503
            ? "智能体服务暂不可用，请稍后重试或重启 API"
            : resp.status === 401
              ? "未授权，请重新登录"
              : `智能体实时通道失败（HTTP ${resp.status}）`,
        );
      }
      const reader = resp.body.getReader();
      const dec = new TextDecoder();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += dec.decode(value, { stream: true });
        for (;;) {
          const idx = buffer.indexOf("\n\n");
          if (idx < 0) break;
          const block = buffer.slice(0, idx);
          buffer = buffer.slice(idx + 2);
          for (const line of block.split("\n")) {
            if (!line.startsWith("data:")) continue;
            const raw = line.slice(5).trim();
            if (!raw) continue;
            try {
              onData(JSON.parse(raw) as AgentStreamPayload);
            } catch {
              /* ignore malformed chunk */
            }
          }
        }
      }
    } catch (e) {
      if (!ac.signal.aborted && onError) {
        onError(e as Error);
      }
    }
  })();

  return () => ac.abort();
}
