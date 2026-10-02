import { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { Client, Project } from "./api/client";

export interface AppCtx {
  api: Client;
  projects: Project[];
  projectId: string;
  setProjectId: (id: string) => void;
  refreshProjects: () => Promise<void>;
  notify: (msg: string, kind?: "ok" | "error") => void;
  logout: () => void;
}

export const Ctx = createContext<AppCtx | null>(null);

export function useApp(): AppCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error("useApp outside provider");
  return c;
}

/** Runs an async loader, re-running whenever deps change; returns data, error and a reload. */
export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(load, deps);
  const reload = useCallback(async () => {
    setLoading(true);
    try {
      setData(await run());
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [run]);
  useEffect(() => {
    void reload();
  }, [reload]);
  return { data, error, loading, reload };
}

export function fmtTime(s: string | null | undefined): string {
  if (!s) return "—";
  const d = new Date(s);
  return d.toLocaleString("zh-CN", { hour12: false });
}
