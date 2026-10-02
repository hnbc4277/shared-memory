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

export function createClient(base: string, getKey: () => string | null, fetchImpl: Fetch = fetch) {
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
  return {
    get: <T>(path: string, query?: Query) => request<T>("GET", path, { query }),
    post: <T>(path: string, body?: unknown) => request<T>("POST", path, { body: body ?? {} }),
    put: <T>(path: string, body: unknown) => request<T>("PUT", path, { body }),
    del: (path: string, headers?: Record<string, string>) => request<void>("DELETE", path, { headers }),
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

export interface Message {
  id: string;
  seq: number;
  role: string;
  agent: string;
  agent_path: string;
  skills: string[];
  plugins: string[];
  tools: string[];
  model: string;
  content: string | null;
  content_stored: boolean;
  attachment_ids: string[];
  created_at: string;
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
}

export interface Policy {
  store_transcript: boolean;
  store_attachments: boolean;
  inject_enabled: boolean;
  vault_enabled: boolean;
  artifact_searchable: boolean;
  retention_days: number;
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
}
