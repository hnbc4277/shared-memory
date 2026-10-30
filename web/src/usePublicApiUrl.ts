import { API_BASE } from "./api/client";
import { useApp, useLoad } from "./ctx";

export type PublicApiUrlState = {
  /** Client-reachable API base from GET /v1/me → public_api_url (API .env PUBLIC_API_URL). */
  url: string;
  loading: boolean;
};

/**
 * Client-reachable API base for MCP / setup guides.
 * Source of truth: API .env / compose.env PUBLIC_API_URL (via GET /v1/me public_api_url).
 * Falls back to the console's API_BASE only after /v1/me returns without public_api_url —
 * never a hardcoded public domain in the UI.
 */
export function usePublicApiUrl(): PublicApiUrlState {
  const { api } = useApp();
  const me = useLoad(
    () => api.get<{ account_id: string; public_api_url?: string }>("/v1/me"),
    [api],
  );
  const fromApi = me.data?.public_api_url?.replace(/\/$/, "").trim() ?? "";
  if (fromApi) return { url: fromApi, loading: false };
  // Keep loading UI until /v1/me settles — avoid flashing API_BASE (often 127.0.0.1).
  if (me.loading || (!me.data && !me.error)) return { url: "", loading: true };
  return { url: API_BASE.replace(/\/$/, ""), loading: false };
}
