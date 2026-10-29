import gsap from "gsap";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { subscribeAgentsStream } from "../agentsStream";
import { useApp } from "../ctx";

interface AgentGroup {
  instance_count: number;
  online_count: number;
}

const TILES = [
  { id: "agents", label: "智能体", hint: "连接与在线", tone: "violet" as const },
  { id: "history", label: "历史", hint: "会话与回执", tone: "blue" as const },
  { id: "memory", label: "记忆", hint: "检索与编辑", tone: "lime" as const },
  { id: "keys", label: "密钥", hint: "API 与回执", tone: "blue" as const },
] as const;

function summarize(groups: AgentGroup[] | null) {
  let online = 0;
  let total = 0;
  for (const g of groups ?? []) {
    online += g.online_count;
    total += g.instance_count;
  }
  return { online, total, offline: Math.max(0, total - online) };
}

function TileIcon({ id }: { id: (typeof TILES)[number]["id"] }) {
  const paths: Record<(typeof TILES)[number]["id"], ReactNode> = {
    agents: (
      <path
        d="M8 3.5 12 6.2v3.6L8 12.5 4 9.8V6.2L8 3.5Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    ),
    history: (
      <>
        <circle cx="8" cy="8" r="5" fill="none" stroke="currentColor" strokeWidth="1.5" />
        <path d="M8 5.5V8l2 1.5" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
      </>
    ),
    memory: (
      <>
        <circle cx="8" cy="8" r="4.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
        <circle cx="8" cy="8" r="1.5" fill="currentColor" />
      </>
    ),
    keys: (
      <>
        <circle cx="6.5" cy="7" r="2.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
        <path d="M8.5 8.5 12.5 12.5M11 11l1.5-.2M11 11l-.2 1.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
      </>
    ),
  };
  return (
    <svg className="module-tile-svg" viewBox="0 0 16 16" width="18" height="18" aria-hidden>
      {paths[id]}
    </svg>
  );
}

type TriageItem = {
  key: string;
  title: string;
  detail: string;
  actionLabel: string;
  tab: string;
  tone: "warn" | "action" | "ok";
};

export default function OverviewPage() {
  const { api, projects, navigateTab, pendingConfirm, openCmdPalette } = useApp();
  const bentoRef = useRef<HTMLDivElement>(null);
  const [groups, setGroups] = useState<AgentGroup[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await api.get<{ groups: AgentGroup[] }>("/v1/agents");
        if (!cancelled) setGroups(res.groups);
      } catch {
        if (!cancelled) setGroups([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api]);

  useEffect(() => {
    return subscribeAgentsStream(
      (payload) => setGroups(payload.groups as AgentGroup[]),
      () => {},
    );
  }, []);
  const { online, total, offline } = useMemo(() => summarize(groups), [groups]);
  const pending = pendingConfirm.vault + pendingConfirm.artifacts;

  const share = useMemo(() => {
    if (total <= 0) return { onlinePct: 0, offlinePct: 0 };
    return {
      onlinePct: Math.round((online / total) * 100),
      offlinePct: Math.round((offline / total) * 100),
    };
  }, [online, offline, total]);

  const triage = useMemo((): TriageItem[] => {
    if (groups === null) return [];
    const items: TriageItem[] = [];
    if (total === 0) {
      items.push({
        key: "empty",
        title: "尚无连接",
        detail: "复制 API Key，在客户端完成一次 ingest",
        actionLabel: "去密钥",
        tab: "keys",
        tone: "action",
      });
    } else if (offline > 0) {
      items.push({
        key: "offline",
        title: `${offline} 个连接离线`,
        detail: "检查 Key / Daemon heartbeat，或在智能体页查看详情",
        actionLabel: "去智能体",
        tab: "agents",
        tone: "warn",
      });
    }
    if (pending > 0) {
      items.push({
        key: "pending",
        title: `待确认 ${pending}`,
        detail: `凭证 ${pendingConfirm.vault} · 成稿 ${pendingConfirm.artifacts}`,
        actionLabel: pendingConfirm.vault > 0 ? "去凭证库" : "去成稿库",
        tab: pendingConfirm.vault > 0 ? "vault" : "artifacts",
        tone: "action",
      });
    }
    if (items.length === 0) {
      items.push({
        key: "ok",
        title: "一切正常",
        detail: "连接在线，没有待确认项",
        actionLabel: "Ctrl K 跳转",
        tab: "__cmd__",
        tone: "ok",
      });
    }
    return items.slice(0, 3);
  }, [groups, total, offline, pending, pendingConfirm.vault, pendingConfirm.artifacts]);

  const tileMeta = useMemo(() => {
    return {
      agents: total === 0 ? "未接入" : offline > 0 ? `${offline} 离线` : `${online} 在线`,
      history: null as string | null,
      memory: null as string | null,
      keys: null as string | null,
    };
  }, [total, offline, online]);

  useEffect(() => {
    const root = bentoRef.current;
    if (!root || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    root.querySelectorAll<HTMLElement>(".share-track i").forEach((bar) => {
      const w = bar.dataset.w || bar.style.width || "0%";
      bar.style.width = w;
      gsap.fromTo(
        bar,
        { scaleX: 0, transformOrigin: "left" },
        { scaleX: 1, duration: 0.9, ease: "expo.out", delay: 0.15 },
      );
    });
  }, [share.onlinePct, share.offlinePct, groups]);

  useEffect(() => {
    const root = bentoRef.current;
    if (!root || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const rows = root.querySelectorAll<HTMLElement>(".triage-row");
    if (!rows.length) return;
    gsap.fromTo(
      rows,
      { opacity: 0, y: 8 },
      { opacity: 1, y: 0, duration: 0.42, stagger: 0.06, ease: "expo.out", delay: 0.08 },
    );
  }, [triage]);

  return (
    <div className="bento-grid reveal" ref={bentoRef}>
      <div className="bento bento-hero lift m-glow">
        <div className="glow-layer" aria-hidden />
        <span className="eyebrow" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-soft)" }}>
          连接概览
        </span>
        {groups === null ? (
          <div className="display-xl" style={{ opacity: 0.4 }}>
            …
          </div>
        ) : total === 0 ? (
          <>
            <div className="display-xl" style={{ fontSize: 28 }}>
              尚无连接
            </div>
            <p style={{ margin: "8px 0 0", fontSize: 13, color: "var(--ink-muted)" }}>
              配置 API 密钥后，在 Cursor / OpenClaw 等客户端接入即可出现在智能体页。
            </p>
          </>
        ) : (
          <>
            <div className="display-xl tnum">
              {online}
              <small> / {total} 在线</small>
            </div>
            <p style={{ margin: "8px 0 0", fontSize: 13, color: "var(--ink-muted)" }}>
              {offline > 0 ? (
                <>
                  <strong style={{ color: "var(--amber-text, #b8860b)" }}>{offline} 离线</strong> · 可在智能体页查看详情
                </>
              ) : (
                <>
                  全部在线 · <span style={{ color: "var(--leaf-text)" }}>SSE 实时</span>
                </>
              )}
            </p>
          </>
        )}
        <div className="hero-actions" style={{ marginTop: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
          <button type="button" className="btn btn-primary" onClick={() => navigateTab("agents")}>
            智能体连接
          </button>
          {pending > 0 && (
            <button
              type="button"
              className="btn btn-secondary m-pill"
              onClick={() => navigateTab(pendingConfirm.vault > 0 ? "vault" : "artifacts")}
            >
              待确认 {pending}
            </button>
          )}
        </div>
      </div>

      <div className="bento bento-side m-surface lift">
        <div>
          <div className="stat-label">待确认候选</div>
          <div className={`stat-value${pending > 0 ? " is-lime" : ""} tnum`}>{pending}</div>
          <p className="bento-caption">凭证 {pendingConfirm.vault} · 成稿 {pendingConfirm.artifacts}</p>
        </div>
        <div>
          <div className="stat-label">项目</div>
          <div className="stat-value tnum">{projects.length}</div>
        </div>
      </div>

      <div className="bento bento-third m-surface lift">
        <div className="stat-label">连接状态占比</div>
        {total === 0 ? (
          <p className="bento-caption" style={{ marginTop: 10 }}>
            有连接后显示在线 / 离线比例
          </p>
        ) : (
          <ul className="share-list" style={{ marginTop: 10 }}>
            <li>
              <span>在线</span>
              <span className="share-track">
                <i data-w={`${share.onlinePct}%`} style={{ width: `${share.onlinePct}%` }} />
              </span>
              <span className="tnum">{share.onlinePct}%</span>
            </li>
            <li>
              <span>离线</span>
              <span className="share-track">
                <i data-w={`${share.offlinePct}%`} style={{ width: `${share.offlinePct}%` }} />
              </span>
              <span className="tnum">{share.offlinePct}%</span>
            </li>
          </ul>
        )}
      </div>

      <div className="bento bento-full m-surface quick-zone">
        <div className="quick-zone-head">
          <div>
            <div className="stat-label">今天先点哪</div>
            <p className="quick-zone-sub">按当前状态给出该管事项，其余用命令面板跳转</p>
          </div>
          <button type="button" className="cmd-chip" onClick={openCmdPalette}>
            跳转任意模块
            <kbd>Ctrl K</kbd>
          </button>
        </div>

        <div className="quick-zone-body">
          <div className="triage-panel">
            {groups === null ? (
              <p className="bento-caption">加载连接状态…</p>
            ) : (
              <ul className="triage-list">
                {triage.map((item) => (
                  <li key={item.key} className={`triage-row tone-${item.tone}`}>
                    <div className="triage-copy">
                      <strong>{item.title}</strong>
                      <span>{item.detail}</span>
                    </div>
                    <button
                      type="button"
                      className="btn btn-secondary m-pill"
                      onClick={() => (item.tab === "__cmd__" ? openCmdPalette() : navigateTab(item.tab))}
                    >
                      {item.actionLabel}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="module-tile-grid">
            {TILES.map((tile) => {
              const meta = tileMeta[tile.id];
              return (
                <button
                  key={tile.id}
                  type="button"
                  className={`module-tile lift tone-${tile.tone}`}
                  onClick={() => navigateTab(tile.id)}
                >
                  <span className="module-tile-ico">
                    <TileIcon id={tile.id} />
                  </span>
                  <span className="module-tile-copy">
                    <strong>{tile.label}</strong>
                    <span className="module-tile-hint">{meta ?? tile.hint}</span>
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
