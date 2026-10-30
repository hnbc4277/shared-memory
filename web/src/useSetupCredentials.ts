import { useApp, useLoad } from "./ctx";

/** Dedicated key baked into the connection setup script (survives create-time modal). */
export function useSetupCredentials(
  connectionId: string,
  apiKeyFromCreate?: string,
): { apiKey: string; apiUrl: string; loading: boolean; missing: boolean } {
  const fromCreate = (apiKeyFromCreate ?? "").trim();
  const { api } = useApp();
  const needFetch = fromCreate.length === 0 && connectionId.length > 0;
  const loaded = useLoad(
    () =>
      needFetch
        ? api.get<{ api_key?: string; api_url?: string }>(
            `/v1/agents/connections/${connectionId}/setup-credentials`,
          )
        : Promise.resolve({ api_key: fromCreate, api_url: "" }),
    [api, connectionId, needFetch, fromCreate],
  );
  if (fromCreate) {
    return { apiKey: fromCreate, apiUrl: "", loading: false, missing: false };
  }
  const key = (loaded.data?.api_key ?? "").trim();
  if (key) return { apiKey: key, apiUrl: (loaded.data?.api_url ?? "").trim(), loading: false, missing: false };
  if (loaded.loading || (!loaded.data && !loaded.error)) {
    return { apiKey: "", apiUrl: "", loading: true, missing: false };
  }
  return { apiKey: "", apiUrl: "", loading: false, missing: true };
}
