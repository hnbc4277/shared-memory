import { API_BASE } from "./api/client";
import { useApp, useLoad } from "./ctx";

/**
 * Client-reachable API base for MCP / setup guides.
 * Source of truth: API .env PUBLIC_API_URL (via GET /v1/me public_api_url).
 * Falls back to the console's API_BASE when unset — never a hardcoded public domain.
 */
export function usePublicApiUrl(): string {
  const { api } = useApp();
  const me = useLoad(
    () => api.get<{ account_id: string; public_api_url?: string }>("/v1/me"),
    [api],
  );
  const fromApi = me.data?.public_api_url?.replace(/\/$/, "").trim();
  if (fromApi) return fromApi;
  return API_BASE.replace(/\/$/, "");
}
