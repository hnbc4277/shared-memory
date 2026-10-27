export const API_BASE: string = import.meta.env?.VITE_API_BASE ?? "http://127.0.0.1:18080";

export type Query = Record<string, string | number | boolean | undefined | null>;

export function buildUrl(base: string, path: string, query?: Query): string {
  const url = base.replace(/\/+$/, "") + "/" + path.replace(/^\/+/, "");
  if (!query) return url;
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === "") continue;
    params.set(k, String(v));
  }
  const qs = params.toString();
  return qs ? `${url}?${qs}` : url;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

type Fetch = typeof fetch;

export type ClientOptions = { credentials?: RequestCredentials };

export function createClient(
  base: string,
  getKey: () => string | null,
  fetchImpl: Fetch = fetch,
  options: ClientOptions = {},
) {
  async function request<T>(
    method: string,
    path: string,
    opts: { query?: Query; body?: unknown; headers?: Record<string, string> } = {},
  ): Promise<T> {
    const headers: Record<string, string> = { ...opts.headers };
    const key = getKey();
    if (key) headers["Authorization"] = `Bearer ${key}`;
    if (opts.body !== undefined) headers["Content-Type"] = "application/json";
    const resp = await fetchImpl(buildUrl(base, path, opts.query), {
      method,
      headers,
      credentials: options.credentials,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
    const text = await resp.text();
    if (!resp.ok) {
      let msg = text || resp.statusText;
      try {
        msg = JSON.parse(text).error ?? msg;
      } catch {
        /* plain-text error body */
      }
      throw new ApiError(resp.status, msg);
    }
    return (text ? JSON.parse(text) : undefined) as T;
  }

  async function postForm<T>(path: string, form: FormData): Promise<T> {
    const headers: Record<string, string> = {};
    const key = getKey();
    if (key) headers["Authorization"] = `Bearer ${key}`;
    const resp = await fetchImpl(buildUrl(base, path), {
      method: "POST",
      headers,
      credentials: options.credentials,
      body: form,
    });
    const text = await resp.text();
    if (!resp.ok) {
      let msg = text || resp.statusText;
      try {
        msg = JSON.parse(text).error ?? msg;
      } catch {
        /* plain */
      }
      throw new ApiError(resp.status, msg);
    }
    return (text ? JSON.parse(text) : undefined) as T;
  }

  async function getBlob(path: string, query?: Query): Promise<Blob> {
    const headers: Record<string, string> = {};
    const key = getKey();
    if (key) headers["Authorization"] = `Bearer ${key}`;
    const resp = await fetchImpl(buildUrl(base, path, query), {
      method: "GET",
      headers,
      credentials: options.credentials,
    });
    if (!resp.ok) {
      const text = await resp.text();
      throw new ApiError(resp.status, text || resp.statusText);
    }
    return resp.blob();
  }

  return {
    get: <T>(path: string, query?: Query) => request<T>("GET", path, { query }),
    post: <T>(path: string, body?: unknown) => request<T>("POST", path, { body: body ?? {} }),
    put: <T>(path: string, body: unknown) => request<T>("PUT", path, { body }),
    patch: <T>(path: string, body: unknown) => request<T>("PATCH", path, { body }),
    del: (path: string, headers?: Record<string, string>) => request<void>("DELETE", path, { headers }),
    postForm,
    getBlob,
  };
}

export type Client = ReturnType<typeof createClient>;

export interface Project {
  id: string;
  name: string;
  created_at: string;
}

export interface Session {
  id: string;
  project_id: string | null;
  agent: string;
  agent_path: string;
  title: string;
  started_at: string;
  last_active_at: string;
  message_count: number;
  agent_paths: string[];
  degraded: boolean;
}

export interface ToolCall {
  kind: string;
  name: string;
  started_at?: string;
  ended_at?: string;
  output?: string;
}

export interface Message {
  id: string;
  seq: number;
  role: string;
  agent: string;
  agent_path: string;
  skills: string[];
  plugins: string[];
  tools: string[];
  tool_calls?: ToolCall[];
  model: string;
  content: string | null;
  content_stored: boolean;
  attachment_ids: string[];
  created_at: string;
}

export interface EntityAlias {
  id: string;
  project_id: string | null;
  canonical: string;
  alias: string;
}

export interface RecallReceipt {
  id: string;
  queried_at: string;
  hit_count: number;
  kb_hit_count?: number;
  degraded: boolean;
  source: string;
  warnings?: string[];
  agent?: string;
  agent_path?: string;
  connection_id?: string | null;
  query_preview?: string;
}

export interface Memory {
  id: string;
  project_id: string | null;
  scope: "global" | "project";
  kind: string;
  body: string;
  source_session_id: string | null;
  source_agent_path: string;
  supersedes: string | null;
  invalidated: boolean;
  updated_at: string;
}

export interface RecallHit {
  id: string;
  doc_type: string;
  kind: string;
  scope: string;
  title?: string;
  body: string;
  source_agent_path: string;
  created_at: string;
  score: number;
}

export interface RecallResult {
  profile: string;
  running_summary: string;
  hits: RecallHit[];
  context: string;
  inject_enabled: boolean;
  warnings: string[];
  receipt_id?: string;
}

export interface Policy {
  store_transcript: boolean;
  store_attachments: boolean;
  inject_enabled: boolean;
  kb_inject_enabled: boolean;
  vault_enabled: boolean;
  artifact_searchable: boolean;
  cloud_vision_enabled: boolean;
  retention_days: number;
}

export interface KBDocument {
  id: string;
  title: string;
  mime: string;
  source_type: "upload" | "artifact" | "attachment";
  bytes: number;
  content_hash: string;
  status: "pending" | "ready" | "failed";
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface VaultCandidate {
  id: string;
  project_id: string | null;
  agent_path: string;
  title: string;
  site_url: string;
  username: string;
  created_at: string;
  expires_at: string;
}

export interface VaultItem {
  id: string;
  project_id: string | null;
  title: string;
  site_url: string;
  username: string;
  source_agent_path: string;
  consent_method: string;
  updated_at: string;
}

export interface ArtifactCandidate {
  id: string;
  agent_path: string;
  title: string;
  type: string;
  summary: string;
  created_at: string;
}

export interface Artifact {
  id: string;
  project_id: string | null;
  title: string;
  type: string;
  summary: string;
  tags: string[];
  current_version: number;
  source_agent_path: string;
  updated_at: string;
}

export interface ArtifactDetail {
  artifact: Artifact;
  body: string;
  versions: { version: number; body_hash: string; created_at: string }[];
}

export interface APIKeyInfo {
  id: string;
  name: string;
  created_at: string;
  connection_id?: string;
  connection_label?: string;
}
