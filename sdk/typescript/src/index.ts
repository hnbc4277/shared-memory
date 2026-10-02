/** Shared Memory REST client (Bearer SHARED_MEMORY_API_KEY). */

export type IngestRequest = {
  session_id?: string;
  external_session_id?: string;
  project_id?: string;
  title?: string;
  agent?: string;
  agent_path?: string;
  skills: string[];
  plugins: string[];
  tools: string[];
  model?: string;
  role: string;
  content: string;
  attachment_ids?: string[];
};

export type IngestResult = {
  session_id: string;
  message_id: string;
  seq: number;
  warnings?: string[];
  prompts?: unknown[];
};

export type RecallRequest = {
  query: string;
  project_id?: string;
  top_k?: number;
};

export type RecallResult = {
  profile: string;
  running_summary: string;
  hits: unknown[];
  context: string;
  inject_enabled: boolean;
  warnings?: string[];
};

export type Session = {
  id: string;
  project_id?: string | null;
  agent?: string;
  agent_path?: string;
  title?: string;
  status?: string;
  started_at?: string;
  last_active_at?: string;
  message_count?: number;
};

export type SessionDetail = {
  session: Session;
  messages: unknown[];
};

export type Candidates = {
  vault: unknown[];
  artifacts: unknown[];
};

export type ListSessionsFilter = {
  project_id?: string;
  agent?: string;
  agent_path?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export type DeviceCodeRequest = {
  client_name: string;
  agent?: string;
};

export type DeviceCode = {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete: string;
  expires_in: number;
  interval: number;
};

export type DeviceToken = {
  api_key: string;
  device_id: string;
  account_id: string;
};

export class APIError extends Error {
  readonly status: number;
  readonly body: string;

  constructor(status: number, body: string) {
    super(`shared memory api: ${status} ${body.trim()}`);
    this.name = "APIError";
    this.status = status;
    this.body = body;
  }
}

export type ClientOptions = {
  baseUrl?: string;
  apiKey?: string;
  fetch?: typeof fetch;
};

export class Client {
  readonly baseUrl: string;
  readonly apiKey: string;
  private readonly fetchFn: typeof fetch;

  constructor(opts: ClientOptions = {}) {
    this.baseUrl = (opts.baseUrl ?? process.env.SHARED_MEMORY_API_URL ?? "").replace(
      /\/$/,
      "",
    );
    this.apiKey = opts.apiKey ?? process.env.SHARED_MEMORY_API_KEY ?? "";
    this.fetchFn = opts.fetch ?? fetch;
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const headers: Record<string, string> = {};
    if (this.apiKey) {
      headers.Authorization = `Bearer ${this.apiKey}`;
    }
    let payload: string | undefined;
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      payload = JSON.stringify(body);
    }
    const res = await this.fetchFn(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: payload,
    });
    const text = await res.text();
    if (!res.ok) {
      throw new APIError(res.status, text);
    }
    if (res.status === 204 || text.length === 0) {
      return undefined as T;
    }
    return JSON.parse(text) as T;
  }

  ingest(req: IngestRequest): Promise<IngestResult> {
    return this.request<IngestResult>("POST", "/v1/ingest", req);
  }

  recall(req: RecallRequest): Promise<RecallResult> {
    return this.request<RecallResult>("POST", "/v1/recall", req);
  }

  listSessions(filter: ListSessionsFilter = {}): Promise<Session[]> {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(filter)) {
      if (v !== undefined && v !== null && v !== "") {
        q.set(k, String(v));
      }
    }
    const qs = q.toString();
    return this.request<Session[]>("GET", `/v1/sessions${qs ? `?${qs}` : ""}`);
  }

  getSession(sessionId: string): Promise<SessionDetail> {
    return this.request<SessionDetail>(
      "GET",
      `/v1/sessions/${encodeURIComponent(sessionId)}`,
    );
  }

  listCandidates(): Promise<Candidates> {
    return this.request<Candidates>("GET", "/v1/candidates");
  }

  resolveCandidate(
    kind: "vault" | "artifact",
    candidateId: string,
    decision: "accept" | "reject",
  ): Promise<unknown> {
    const prefix =
      kind === "artifact" ? "/v1/artifacts/candidates/" : "/v1/vault/candidates/";
    return this.request(
      "POST",
      `${prefix}${encodeURIComponent(candidateId)}/${decision}`,
      {},
    );
  }

  createDeviceCode(req: DeviceCodeRequest): Promise<DeviceCode> {
    return this.request<DeviceCode>("POST", "/v1/device/code", req);
  }

  pollDeviceToken(deviceCode: string): Promise<DeviceToken> {
    return this.request<DeviceToken>("POST", "/v1/device/token", {
      device_code: deviceCode,
    });
  }

  heartbeat(daemonVersion = "", spoolPending = 0): Promise<void> {
    return this.request("POST", "/v1/devices/heartbeat", {
      daemon_version: daemonVersion,
      spool_pending: spoolPending,
    });
  }
}
