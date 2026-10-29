import { useCallback, useEffect, useMemo, useRef, useState, type ComponentType } from "react";
import { API_BASE, createClient, type Project } from "./api/client";
import { Ctx, type AppCtx } from "./ctx";
import CmdPalette, { type CmdNavItem } from "./CmdPalette";
import Login from "./pages/Login";
import OverviewPage from "./pages/OverviewPage";
import HistoryPage from "./pages/HistoryPage";
import MemoryPage from "./pages/MemoryPage";
import VaultPage from "./pages/VaultPage";
import ArtifactsPage from "./pages/ArtifactsPage";
import KBPage from "./pages/KBPage";
import ProjectsPage from "./pages/ProjectsPage";
import SettingsPage from "./pages/SettingsPage";
import KeysPage from "./pages/KeysPage";
import DevicePage from "./pages/DevicePage";
import AgentsPage from "./pages/AgentsPage";
import ShellOverview from "./ShellOverview";
import { installPearlMicroInteractions } from "./pearl/interactions";
import { pageEnter, playShellEntrance, slideNavThumb } from "./pearl/motion";

const KEY_STORAGE = "sm.apiKey";

type TabId =
  | "overview"
  | "history"
  | "agents"
  | "memory"
  | "vault"
  | "artifacts"
  | "kb"
  | "projects"
  | "settings"
  | "keys"
  | "device";

const NAV_GROUPS: { title: string; items: { id: TabId; label: string; icon: string }[] }[] = [
  {
    title: "会话与连接",
    items: [
      { id: "overview", label: "总览", icon: "▣" },
      { id: "history", label: "历史", icon: "◷" },
      { id: "memory", label: "记忆", icon: "◉" },
      { id: "agents", label: "智能体", icon: "⎔" },
    ],
  },
  {
    title: "资产",
    items: [
      { id: "vault", label: "凭证库", icon: "⌁" },
      { id: "artifacts", label: "成稿库", icon: "▣" },
      { id: "kb", label: "知识库", icon: "▦" },
    ],
  },
  {
    title: "管理",
    items: [
      { id: "projects", label: "项目", icon: "◇" },
      { id: "keys", label: "密钥与回执", icon: "🔑" },
      { id: "device", label: "授权设备", icon: "📱" },
      { id: "settings", label: "设置", icon: "⚙" },
    ],
  },
];

const PAGES: Record<TabId, ComponentType> = {
  overview: OverviewPage,
  history: HistoryPage,
  agents: AgentsPage,
  memory: MemoryPage,
  vault: VaultPage,
  artifacts: ArtifactsPage,
  kb: KBPage,
  projects: ProjectsPage,
  settings: SettingsPage,
  keys: KeysPage,
  device: DevicePage,
};

const TAB_LABEL: Record<TabId, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.items.map((i) => [i.id, i.label])),
) as Record<TabId, string>;

function isTabId(id: string): id is TabId {
  return id in PAGES;
}

export default function App() {
  const [apiKey, setApiKey] = useState<string | null>(() => localStorage.getItem(KEY_STORAGE));
  const [tab, setTab] = useState<TabId>(() => {
    const h = location.hash.slice(1);
    return isTabId(h) ? h : "overview";
  });
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState("");
  const [toast, setToast] = useState<{ msg: string; kind: "ok" | "error" } | null>(null);
  const [cmdOpen, setCmdOpen] = useState(false);
  const [pendingConfirm, setPendingConfirm] = useState({ vault: 0, artifacts: 0 });

  const frameRef = useRef<HTMLDivElement>(null);
  const workspaceRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLElement>(null);
  const pageRef = useRef<HTMLDivElement>(null);
  const railNavRef = useRef<HTMLElement>(null);
  const enteredRef = useRef(false);

  const api = useMemo(() => createClient(API_BASE, () => apiKey), [apiKey]);

  const notify = useCallback((msg: string, kind: "ok" | "error" = "ok") => {
    setToast({ msg, kind });
    window.setTimeout(() => setToast(null), 3500);
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem(KEY_STORAGE);
    setApiKey(null);
  }, []);

  const openCmdPalette = useCallback(() => setCmdOpen(true), []);

  const refreshProjects = useCallback(async () => {
    try {
      setProjects(await api.get<Project[]>("/v1/projects"));
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }, [api, notify]);

  const refreshPending = useCallback(async () => {
    try {
      const [vault, artifacts] = await Promise.all([
        api.get<unknown[]>("/v1/vault/candidates").catch(() => []),
        api.get<unknown[]>("/v1/artifacts/candidates").catch(() => []),
      ]);
      setPendingConfirm({
        vault: Array.isArray(vault) ? vault.length : 0,
        artifacts: Array.isArray(artifacts) ? artifacts.length : 0,
      });
    } catch {
      /* ignore */
    }
  }, [api]);

  useEffect(() => {
    if (apiKey) {
      void refreshProjects();
      void refreshPending();
    }
  }, [apiKey, refreshProjects, refreshPending]);

  useEffect(() => {
    location.hash = tab;
  }, [tab]);

  useEffect(() => {
    const onHash = () => {
      const id = location.hash.slice(1);
      if (isTabId(id)) setTab(id);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCmdOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (!apiKey) {
      enteredRef.current = false;
      return;
    }
    const frame = frameRef.current;
    if (!frame) return;
    if (!enteredRef.current) {
      playShellEntrance(frame);
      enteredRef.current = true;
    }
    return installPearlMicroInteractions({
      root: frame,
      scrollEl: scrollRef.current,
      workspaceEl: workspaceRef.current,
    });
  }, [apiKey]);

  useEffect(() => {
    if (!apiKey) return;
    const rail = railNavRef.current;
    const active = rail?.querySelector<HTMLElement>(".nav-item.is-active") ?? null;
    const list = active?.closest(".nav-list") as HTMLElement | null;
    const thumb = list?.querySelector<HTMLElement>(".nav-thumb") ?? null;
    if (rail) {
      rail.querySelectorAll(".nav-thumb").forEach((el) => {
        (el as HTMLElement).style.opacity = el === thumb ? "1" : "0";
      });
    }
    slideNavThumb(thumb, list, active);
  }, [apiKey, tab]);

  useEffect(() => {
    if (!apiKey) return;
    pageEnter(pageRef.current);
  }, [apiKey, tab]);

  const navigateTab = useCallback(
    (
      id: string,
      opts?: {
        historyAgent?: string;
        historyConnectionId?: string;
        receiptConnectionId?: string;
        agentsConnectionId?: string;
      },
    ) => {
      if (opts?.historyAgent) sessionStorage.setItem("sm.historyAgent", opts.historyAgent);
      if (opts?.historyConnectionId) sessionStorage.setItem("sm.historyConnectionId", opts.historyConnectionId);
      if (opts?.receiptConnectionId) sessionStorage.setItem("sm.receiptConnectionId", opts.receiptConnectionId);
      if (opts?.agentsConnectionId) sessionStorage.setItem("sm.agentsConnectionId", opts.agentsConnectionId);
      if (isTabId(id)) setTab(id);
    },
    [],
  );

  const cmdItems: CmdNavItem[] = useMemo(
    () =>
      NAV_GROUPS.flatMap((g) =>
        g.items.map((it) => ({ id: it.id, label: it.label, icon: it.icon, group: g.title })),
      ),
    [],
  );

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

  const ctx: AppCtx = {
    api,
    projects,
    projectId,
    setProjectId,
    refreshProjects,
    notify,
    logout,
    navigateTab,
    pendingConfirm,
    openCmdPalette,
  };
  const Page = PAGES[tab] ?? OverviewPage;
  const pendingTotal = pendingConfirm.vault + pendingConfirm.artifacts;

  return (
    <Ctx.Provider value={ctx}>
      <div className="stage">
        <div className="app-frame m-rail" id="app-frame" ref={frameRef}>
          <aside className="rail m-rail">
            <div className="rail-brand">
              <span className="brand-mark">
                <span className="brand-dot" />
              </span>
              <div className="brand-text">
                <div className="brand-name">共享记忆</div>
                <div className="brand-sub">CONSOLE</div>
              </div>
            </div>
            <nav className="rail-nav" ref={railNavRef}>
              {NAV_GROUPS.map((group) => (
                <div className="nav-group" key={group.title}>
                  <div className="nav-group-title">{group.title}</div>
                  <div className="nav-list">
                    <div className="nav-thumb m-pill" aria-hidden />
                    {group.items.map((it) => (
                      <button
                        key={it.id}
                        type="button"
                        className={`nav-item${tab === it.id ? " is-active" : ""}`}
                        onClick={() => setTab(it.id)}
                      >
                        <span className="nav-ico">{it.icon}</span>
                        {it.label}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </nav>
          </aside>

          <div className="workspace" ref={workspaceRef}>
            <header className="topbar">
              <div className="crumbs">
                <span>控制台</span> <span>/</span> <span className="crumb-current">{TAB_LABEL[tab]}</span>
              </div>
              <div className="spacer" />
              <button type="button" className="cmd-trigger m-pill" onClick={openCmdPalette}>
                <span>搜索页面</span>
                <kbd>Ctrl K</kbd>
              </button>
              {pendingTotal > 0 && (
                <button
                  type="button"
                  className="btn btn-secondary m-pill"
                  style={{ height: 34 }}
                  onClick={() => navigateTab(pendingConfirm.vault > 0 ? "vault" : "artifacts")}
                >
                  待确认 {pendingTotal}
                </button>
              )}
              <label className="topbar-project">
                <select value={projectId} onChange={(e) => setProjectId(e.target.value)}>
                  <option value="">全部 / 全局</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <button type="button" className="btn btn-quiet" style={{ height: 34 }} onClick={logout}>
                退出
              </button>
            </header>

            <main className="page-scroll" ref={scrollRef}>
              <div className="page" id="page" ref={pageRef} key={tab}>
                {tab !== "overview" && (
                  <div className="reveal" style={{ marginBottom: 14 }}>
                    <ShellOverview api={api} onGoAgents={() => setTab("agents")} />
                  </div>
                )}
                {tab === "overview" ? (
                  <>
                    <div className="page-head reveal">
                      <div>
                        <h1 className="page-title">总览</h1>
                        <p className="page-lead">连接状态、待确认项与常用入口。</p>
                      </div>
                    </div>
                    <OverviewPage />
                  </>
                ) : (
                  <Page />
                )}
              </div>
            </main>
          </div>
        </div>
      </div>

      <CmdPalette open={cmdOpen} onOpenChange={setCmdOpen} items={cmdItems} onPick={(id) => navigateTab(id)} />
      {toast && <div className={`toast toast-${toast.kind}`}>{toast.msg}</div>}
    </Ctx.Provider>
  );
}
