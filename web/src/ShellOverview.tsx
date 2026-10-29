import { useEffect, useMemo, useRef, useState } from "react";
import { subscribeAgentsStream } from "./agentsStream";
import type { createClient } from "./api/client";

type Api = ReturnType<typeof createClient>;

interface AgentGroup {
  instance_count: number;
  online_count: number;
}

function summarize(groups: AgentGroup[] | null) {
  let online = 0;
  let total = 0;
  for (const g of groups ?? []) {
    online += g.online_count;
    total += g.instance_count;
  }
  return {
    online,
    total,
    offline: Math.max(0, total - online),
    products: groups?.length ?? 0,
  };
}

export default function ShellOverview({
  api,
  onGoAgents,
}: {
  api: Api;
  onGoAgents: () => void;
}) {
  const [groups, setGroups] = useState<AgentGroup[] | null>(null);
  const [err, setErr] = useState(false);
  const [flash, setFlash] = useState(false);
  const prevOnline = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.get<{ groups: AgentGroup[] }>("/v1/agents");
        if (!cancelled) {
          setGroups(res.groups);
          setErr(false);
        }
      } catch {
        if (!cancelled) setErr(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api]);

  useEffect(() => {
    return subscribeAgentsStream(
      (payload) => setGroups(payload.groups as AgentGroup[]),
      () => setErr(true),
    );
  }, []);

  const { online, total, offline } = useMemo(() => summarize(groups), [groups]);

  useEffect(() => {
    if (prevOnline.current !== null && prevOnline.current !== online) {
      setFlash(true);
      const t = window.setTimeout(() => setFlash(false), 600);
      prevOnline.current = online;
      return () => window.clearTimeout(t);
    }
    prevOnline.current = online;
  }, [online]);

  if (err && groups === null) {
    return (
      <div className="conclusion-bar m-surface reveal lift muted">
        暂时无法汇总连接状态 — 进入各模块仍可正常使用
      </div>
    );
  }

  if (groups === null) {
    return <div className="conclusion-bar m-surface skeleton-bar" aria-hidden />;
  }

  if (total === 0) {
    return (
      <div className="conclusion-bar m-surface reveal lift">
        <span className="conclusion-main">还没有任何连接记录</span>
        <span className="conclusion-sep">·</span>
        <button type="button" className="conclusion-action" onClick={onGoAgents}>
          去配置 API 密钥或添加连接
        </button>
      </div>
    );
  }

  return (
    <div className={`conclusion-bar m-surface reveal lift${flash ? " conclusion-flash" : ""}`}>
      <span className="conclusion-main tnum">
        <strong>{online}</strong>/{total} 连接在线
      </span>
      {offline > 0 && (
        <>
          <span className="conclusion-sep">·</span>
          <span className="conclusion-warn tnum">{offline} 离线</span>
          <button type="button" className="conclusion-action" onClick={onGoAgents}>
            查看智能体
          </button>
        </>
      )}
      {offline === 0 && online > 0 && (
        <>
          <span className="conclusion-sep">·</span>
          <span className="conclusion-ok">全部在线</span>
        </>
      )}
      <span className="conclusion-live" title="服务端推送实时更新">实时</span>
    </div>
  );
}
