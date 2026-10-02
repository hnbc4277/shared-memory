import { useCallback, useEffect, useMemo, useState } from "react";
import { API_BASE, createClient, type Project } from "./api/client";
import { Ctx, type AppCtx } from "./ctx";
import Login from "./pages/Login";
import HistoryPage from "./pages/HistoryPage";
import MemoryPage from "./pages/MemoryPage";
import VaultPage from "./pages/VaultPage";
import ArtifactsPage from "./pages/ArtifactsPage";
import ProjectsPage from "./pages/ProjectsPage";
import SettingsPage from "./pages/SettingsPage";
import KeysPage from "./pages/KeysPage";
import DevicePage from "./pages/DevicePage";

const KEY_STORAGE = "sm.apiKey";

const TABS = [
  { id: "history", label: "历史", page: HistoryPage },
  { id: "memory", label: "记忆", page: MemoryPage },
  { id: "vault", label: "凭证库", page: VaultPage },
  { id: "artifacts", label: "成稿库", page: ArtifactsPage },
  { id: "projects", label: "项目", page: ProjectsPage },
  { id: "settings", label: "设置", page: SettingsPage },
  { id: "keys", label: "密钥", page: KeysPage },
  { id: "device", label: "授权设备", page: DevicePage },
] as const;

type TabId = (typeof TABS)[number]["id"];

export default function App() {
  const [apiKey, setApiKey] = useState<string | null>(() => localStorage.getItem(KEY_STORAGE));
  const [tab, setTab] = useState<TabId>(() => (location.hash.slice(1) as TabId) || "history");
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState("");
  const [toast, setToast] = useState<{ msg: string; kind: "ok" | "error" } | null>(null);

  const api = useMemo(() => createClient(API_BASE, () => apiKey), [apiKey]);

  const notify = useCallback((msg: string, kind: "ok" | "error" = "ok") => {
    setToast({ msg, kind });
    window.setTimeout(() => setToast(null), 3500);
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem(KEY_STORAGE);
    setApiKey(null);
  }, []);

  const refreshProjects = useCallback(async () => {
    try {
      setProjects(await api.get<Project[]>("/v1/projects"));
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }, [api, notify]);

  useEffect(() => {
    if (apiKey) void refreshProjects();
  }, [apiKey, refreshProjects]);

  useEffect(() => {
    location.hash = tab;
  }, [tab]);

  useEffect(() => {
    const onHash = () => {
      const id = location.hash.slice(1) as TabId;
      if (TABS.some((t) => t.id === id)) setTab(id);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  if (!apiKey) {
    return (
      <Login
        onKey={(k) => {
          localStorage.setItem(KEY_STORAGE, k);
          setApiKey(k);
        }}
      />
    );
  }

  const ctx: AppCtx = { api, projects, projectId, setProjectId, refreshProjects, notify, logout };
  const Page = TABS.find((t) => t.id === tab)?.page ?? HistoryPage;

  return (
    <Ctx.Provider value={ctx}>
      <div className="shell">
        <aside className="sidebar">
          <div className="brand">
            <span className="brand-mark" aria-hidden>◎</span>
            <div>
              <strong>共享记忆</strong>
              <small>Shared Memory</small>
            </div>
          </div>
          <nav>
            {TABS.filter((t) => t.id !== "device").map((t) => (
              <button key={t.id} className={t.id === tab ? "nav active" : "nav"} onClick={() => setTab(t.id)}>
                {t.label}
              </button>
            ))}
          </nav>
          <div className="sidebar-foot">
            <label className="field">
              <span>当前项目</span>
              <select value={projectId} onChange={(e) => setProjectId(e.target.value)}>
                <option value="">全部 / 全局</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            </label>
            <button className="ghost" onClick={logout}>退出登录</button>
          </div>
        </aside>
        <main className="content">
          <Page />
        </main>
        {toast && <div className={`toast toast-${toast.kind}`}>{toast.msg}</div>}
      </div>
    </Ctx.Provider>
  );
}
