import { useState } from "react";
import type { Message, RecallReceipt, Session, ToolCall } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Chip, ChipList, Empty, ErrorLine, Section } from "../ui";

function toRFC3339Date(d: string, endOfDay: boolean): string | undefined {
  if (!d) return undefined;
  return endOfDay ? `${d}T23:59:59Z` : `${d}T00:00:00Z`;
}

export default function HistoryPage() {
  const { api, projectId, projects } = useApp();
  const [agentPath, setAgentPath] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [applied, setApplied] = useState({ agent: "", from: "", to: "" });
  const [selected, setSelected] = useState<string | null>(null);

  const sessions = useLoad(
    () => api.get<Session[]>("/v1/sessions", {
      project_id: projectId,
      agent_path: applied.agent,
      from: toRFC3339Date(applied.from, false),
      to: toRFC3339Date(applied.to, true),
      limit: 100,
    }),
    [api, projectId, applied],
  );
  const projectName = (id: string | null) => (id ? projects.find((p) => p.id === id)?.name ?? "项目" : "全局");

  return (
    <div className="split">
      <Section
        title="会话历史"
        actions={
          <form className="row wrap" onSubmit={(e) => {
            e.preventDefault();
            setApplied({ agent: agentPath.trim(), from, to });
          }}>
            <input value={agentPath} onChange={(e) => setAgentPath(e.target.value)} placeholder="智能体路径" className="wide" />
            <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} title="从" />
            <input type="date" value={to} onChange={(e) => setTo(e.target.value)} title="到" />
            <button>过滤</button>
          </form>
        }
      >
        <ErrorLine error={sessions.error} />
        {sessions.data?.length === 0 && <Empty>还没有会话。把 API Key 配置到 Cursor / Codex / 小龙虾等智能体后，聊天会自动出现在这里。</Empty>}
        <ul className="list">
          {sessions.data?.map((s) => (
            <li key={s.id} className={selected === s.id ? "item selected" : "item"} onClick={() => setSelected(s.id)}>
              <div className="item-title">
                <span>{s.title || "未命名会话"}</span>
                {s.degraded && <Chip tone="warn" title="写入时队列不可用，语义记忆可能缺失">降级</Chip>}
              </div>
              <div className="item-meta">
                {s.agent_paths.map((p) => <Chip key={p} tone="agent">{p}</Chip>)}
                <span className="muted">{projectName(s.project_id)}</span>
                <span className="muted">{s.message_count} 条 · {fmtTime(s.last_active_at)}</span>
              </div>
            </li>
          ))}
        </ul>
      </Section>
      {selected ? <SessionDetail id={selected} /> : <Section title="会话详情"><Empty>选择左侧会话查看每条消息的来源智能体、技能、插件与模型。</Empty></Section>}
    </div>
  );
}

function SessionDetail({ id }: { id: string }) {
  const { api } = useApp();
  const detail = useLoad(() => api.get<{ session: Session; messages: Message[] }>(`/v1/sessions/${id}`), [api, id]);
  const receipts = useLoad(
    () => api.get<RecallReceipt[]>("/v1/recall/receipts", { session_id: id, limit: 5 }),
    [api, id],
  );
  const s = detail.data?.session;
  return (
    <Section title={s?.title || "会话详情"}>
      <ErrorLine error={detail.error} />
      {s && (
        <p className="muted small">
          开始于 {fmtTime(s.started_at)} · 主智能体 <Chip tone="agent">{s.agent}</Chip>
        </p>
      )}
      {!!receipts.data?.length && (
        <div className="receipts">
          <strong className="small">最近召回</strong>
          <ul className="muted small">
            {receipts.data.map((r) => (
              <li key={r.id}>
                {fmtTime(r.queried_at)} · hits={r.hit_count} · {r.source}
                {r.degraded ? " · 降级" : ""}
                {r.warnings?.length ? ` · ${r.warnings.join(",")}` : ""}
              </li>
            ))}
          </ul>
        </div>
      )}
      <ol className="messages">
        {detail.data?.messages.map((m) => (
          <li key={m.id} className={`msg msg-${m.role}`}>
            <div className="msg-head">
              <strong>{m.role === "user" ? "用户" : m.role === "assistant" ? "助手" : m.role}</strong>
              <Chip tone="agent">{m.agent_path}</Chip>
              <span className="muted">{m.model}</span>
              <span className="muted">{fmtTime(m.created_at)}</span>
            </div>
            <div className="msg-prov">
              <ChipList label="技能" items={m.skills} tone="skill" />
              <ChipList label="插件" items={m.plugins} tone="plugin" />
              <ChipList label="工具" items={m.tools} tone="tool" />
            </div>
            {!!m.tool_calls?.length && <ToolCallList calls={m.tool_calls} />}
            {!!m.attachment_ids?.length && <AttachmentThumbs ids={m.attachment_ids} />}
            {m.content_stored && m.content !== null ? (
              <pre className="msg-body">{m.content}</pre>
            ) : (
              <div className="muted small">原文未保存（策略关闭了原文存储），仅保留溯源信息。</div>
            )}
          </li>
        ))}
      </ol>
    </Section>
  );
}

function ToolCallList({ calls }: { calls: ToolCall[] }) {
  return (
    <ul className="tool-calls">
      {calls.map((tc, i) => (
        <li key={i}>
          <Chip tone="tool">{tc.kind}:{tc.name}</Chip>
          <span className="muted small"> {tc.started_at}{tc.ended_at ? ` → ${tc.ended_at}` : ""}</span>
        </li>
      ))}
    </ul>
  );
}

function AttachmentThumbs({ ids }: { ids: string[] }) {
  return (
    <div className="thumbs row">
      {ids.map((id) => <Thumb key={id} id={id} />)}
    </div>
  );
}

function Thumb({ id }: { id: string }) {
  const { api } = useApp();
  const thumb = useLoad(() => api.get<{ thumbnail_url: string | null }>(`/v1/attachments/${id}/thumbnail`), [api, id]);
  if (!thumb.data?.thumbnail_url) return null;
  return <img className="thumb" src={thumb.data.thumbnail_url} alt="" width={96} height={96} />;
}
