import { useState } from "react";
import type { EntityAlias, Memory, RecallResult } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Chip, Empty, ErrorLine, Section } from "../ui";

const KINDS: Record<string, string> = {
  "": "全部",
  preference: "偏好",
  decision: "决策",
  fact: "事实",
  entity: "实体",
  episode_summary: "会话摘要",
  running_summary: "项目近况",
  attachment_text: "附件内容",
};

export default function MemoryPage() {
  const { api, projectId, notify } = useApp();
  const [kind, setKind] = useState("");
  const [query, setQuery] = useState("");
  const [recall, setRecall] = useState<RecallResult | null>(null);
  const [editing, setEditing] = useState<{ id: string; body: string } | null>(null);

  const memories = useLoad(() => api.get<Memory[]>("/v1/memories", { project_id: projectId, kind, limit: 200 }), [api, projectId, kind]);
  const profile = useLoad(() => api.get<{ body: string; updated_at: string }>("/v1/profile"), [api]);
  const entities = useLoad(() => api.get<EntityAlias[]>("/v1/entities", { project_id: projectId }), [api, projectId]);

  async function runRecall() {
    try {
      setRecall(await api.post<RecallResult>("/v1/recall", { query, project_id: projectId || undefined, top_k: 8 }));
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function saveEdit() {
    if (!editing) return;
    try {
      await api.put(`/v1/memories/${editing.id}`, { body: editing.body });
      setEditing(null);
      notify("已更正：旧记忆失效，新记忆生效");
      await Promise.all([memories.reload(), profile.reload()]);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function invalidate(id: string) {
    if (!confirm("让这条记忆失效？之后不会再被召回。")) return;
    try {
      await api.del(`/v1/memories/${id}`);
      await Promise.all([memories.reload(), profile.reload()]);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  return (
    <div className="stack">
      <Section title="召回测试" actions={<span className="muted small">模拟智能体开始任务时注入的上下文</span>}>
        <form className="row" onSubmit={(e) => { e.preventDefault(); void runRecall(); }}>
          <input className="wide" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="输入问题，例如：我们的向量库用的是什么？" />
          <button className="primary" disabled={!query.trim()}>召回</button>
        </form>
        {recall && (
          <div className="recall">
            {recall.warnings.length > 0 && <div className="muted small">提示：{recall.warnings.join("、")}</div>}
            {recall.hits.length === 0 && <Empty>没有命中。</Empty>}
            {recall.hits.map((h) => (
              <div key={h.id} className="hit">
                <Chip>{h.doc_type === "artifact" ? `成稿 · ${h.title}` : KINDS[h.kind] ?? h.kind}</Chip>
                <Chip tone="agent">{h.source_agent_path || "—"}</Chip>
                <span>{h.body}</span>
              </div>
            ))}
            {recall.context && (
              <details>
                <summary>注入给智能体的上下文</summary>
                <pre className="msg-body">{recall.context}</pre>
              </details>
            )}
          </div>
        )}
      </Section>

      <Section title="用户画像（全局）" actions={<span className="muted small">由偏好类记忆自动生成 · {fmtTime(profile.data?.updated_at)}</span>}>
        {profile.data?.body ? <pre className="msg-body">{profile.data.body}</pre> : <Empty>尚未形成画像。</Empty>}
      </Section>

      <Section title="实体别名" actions={<span className="muted small">召回时会把别名展开为规范名</span>}>
        <ErrorLine error={entities.error} />
        {entities.data?.length === 0 && <Empty>暂无别名。</Empty>}
        {!!entities.data?.length && (
          <table className="table">
            <thead><tr><th>规范名</th><th>别名</th></tr></thead>
            <tbody>
              {entities.data.map((e) => (
                <tr key={e.id}><td>{e.canonical}</td><td>{e.alias}</td></tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section
        title="记忆"
        actions={
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            {Object.entries(KINDS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
        }
      >
        <ErrorLine error={memories.error} />
        {memories.data?.length === 0 && <Empty>暂无记忆。</Empty>}
        <ul className="list">
          {memories.data?.map((m) => (
            <li key={m.id} className="item static">
              <div className="item-meta">
                <Chip>{KINDS[m.kind] ?? m.kind}</Chip>
                <Chip tone={m.scope === "global" ? "neutral" : "skill"}>{m.scope === "global" ? "全局" : "项目"}</Chip>
                <Chip tone="agent">{m.source_agent_path || "—"}</Chip>
                <span className="muted">{fmtTime(m.updated_at)}</span>
                <span className="spacer" />
                <button className="small" onClick={() => setEditing({ id: m.id, body: m.body })}>更正</button>
                <button className="small danger" onClick={() => void invalidate(m.id)}>失效</button>
              </div>
              {editing?.id === m.id ? (
                <div className="stack-tight">
                  <textarea rows={3} value={editing.body} onChange={(e) => setEditing({ ...editing, body: e.target.value })} />
                  <div className="row">
                    <button className="primary" onClick={() => void saveEdit()}>保存更正</button>
                    <button onClick={() => setEditing(null)}>取消</button>
                  </div>
                </div>
              ) : (
                <pre className="msg-body">{m.body}</pre>
              )}
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}
