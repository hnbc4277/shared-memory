import { useState } from "react";
import { API_BASE, buildUrl, type Artifact, type ArtifactCandidate, type ArtifactDetail } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Chip, Empty, ErrorLine, Section } from "../ui";

const TYPES: Record<string, string> = { "": "全部类型", skill: "技能", doc: "文档", note: "笔记", checklist: "清单", other: "其他" };

export default function ArtifactsPage() {
  const { api, projectId, notify } = useApp();
  const [type, setType] = useState("");
  const [tag, setTag] = useState("");
  const [openId, setOpenId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const candidates = useLoad(() => api.get<ArtifactCandidate[]>("/v1/artifacts/candidates"), [api]);
  const list = useLoad(() => api.get<Artifact[]>("/v1/artifacts", { project_id: projectId, type, tag }), [api, projectId, type, tag]);

  async function resolve(id: string, decision: "accept" | "reject") {
    try {
      await api.post(`/v1/artifacts/candidates/${id}/${decision}`);
      notify(decision === "accept" ? "已保存到成稿库" : "已丢弃");
      await Promise.all([candidates.reload(), list.reload()]);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  return (
    <div className="stack">
      <Section title="待确认" actions={<span className="muted small">AI 生成的较长内容（学习技能、文档等），同意后才会保存</span>}>
        <ErrorLine error={candidates.error} />
        {candidates.data?.length === 0 && <Empty>没有待确认的成稿。</Empty>}
        <ul className="list">
          {candidates.data?.map((c) => (
            <li key={c.id} className="item static">
              <div className="item-meta">
                <Chip>{TYPES[c.type] ?? c.type}</Chip>
                <strong>{c.title}</strong>
                <Chip tone="agent">{c.agent_path}</Chip>
                <span className="spacer" />
                <button className="primary small" onClick={() => void resolve(c.id, "accept")}>保存</button>
                <button className="small" onClick={() => void resolve(c.id, "reject")}>不保存</button>
              </div>
              <div className="muted small">{c.summary}</div>
            </li>
          ))}
        </ul>
      </Section>

      <Section
        title="成稿库"
        actions={
          <>
            <select value={type} onChange={(e) => setType(e.target.value)}>
              {Object.entries(TYPES).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select>
            <input value={tag} onChange={(e) => setTag(e.target.value.trim())} placeholder="标签" />
            <button onClick={() => setCreating((v) => !v)}>{creating ? "取消" : "新建"}</button>
          </>
        }
      >
        {creating && <Editor onDone={async () => { setCreating(false); await list.reload(); }} />}
        <ErrorLine error={list.error} />
        {list.data?.length === 0 && <Empty>成稿库为空。</Empty>}
        <ul className="list">
          {list.data?.map((a) => (
            <li key={a.id} className={openId === a.id ? "item selected" : "item"} onClick={() => setOpenId(openId === a.id ? null : a.id)}>
              <div className="item-meta">
                <Chip>{TYPES[a.type] ?? a.type}</Chip>
                <strong>{a.title}</strong>
                {a.tags.map((t) => <Chip key={t} tone="skill">#{t}</Chip>)}
                <span className="muted">v{a.current_version} · {fmtTime(a.updated_at)}</span>
              </div>
              <div className="muted small">{a.summary}</div>
              {openId === a.id && <Viewer id={a.id} onChanged={() => void list.reload()} />}
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}

function Viewer({ id, onChanged }: { id: string; onChanged: () => void }) {
  const { api, notify } = useApp();
  const detail = useLoad(() => api.get<ArtifactDetail>(`/v1/artifacts/${id}`), [api, id]);
  const [editing, setEditing] = useState(false);

  async function exportMd() {
    try {
      const resp = await fetch(buildUrl(API_BASE, `/v1/artifacts/${id}/export`), {
        headers: { Authorization: `Bearer ${localStorage.getItem("sm.apiKey") ?? ""}` },
      });
      if (!resp.ok) throw new Error(await resp.text());
      const blob = await resp.blob();
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `${detail.data?.artifact.title ?? "artifact"}.md`;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function addToKB() {
    try {
      await api.post("/v1/kb/documents", { artifact_id: id });
      notify("已加入知识库（解析中）");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function remove() {
    if (!confirm("删除这篇成稿及其全部版本？")) return;
    try {
      await api.del(`/v1/artifacts/${id}`);
      onChanged();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  if (!detail.data) return <ErrorLine error={detail.error} />;
  const d = detail.data;
  return (
    <div className="viewer" onClick={(e) => e.stopPropagation()}>
      <div className="row">
        <button className="small" onClick={() => setEditing((v) => !v)}>{editing ? "取消编辑" : "编辑"}</button>
        <button className="small" onClick={() => void exportMd()}>导出 Markdown</button>
        <button className="small" onClick={() => void addToKB()}>加入知识库</button>
        <button className="small danger" onClick={() => void remove()}>删除</button>
        <span className="muted small">来源 {d.artifact.source_agent_path || "手动"} · 共 {d.versions.length} 个版本</span>
      </div>
      {editing ? (
        <Editor initial={d} onDone={async () => { setEditing(false); await detail.reload(); onChanged(); }} />
      ) : (
        <pre className="msg-body doc">{d.body}</pre>
      )}
    </div>
  );
}

function Editor({ initial, onDone }: { initial?: ArtifactDetail; onDone: () => Promise<void> }) {
  const { api, projectId, notify } = useApp();
  const [title, setTitle] = useState(initial?.artifact.title ?? "");
  const [type, setType] = useState(initial?.artifact.type ?? "note");
  const [tags, setTags] = useState(initial?.artifact.tags.join(", ") ?? "");
  const [body, setBody] = useState(initial?.body ?? "");

  async function save(opts: { acknowledgeSecret?: boolean; moveSecretsToVault?: boolean } = {}) {
    const payload = {
      title, type, body,
      acknowledge_secret: !!opts.acknowledgeSecret,
      move_secrets_to_vault: !!opts.moveSecretsToVault,
      tags: tags.split(/[,，]/).map((t) => t.trim()).filter(Boolean),
    };
    try {
      if (initial) await api.put(`/v1/artifacts/${initial.artifact.id}`, payload);
      else await api.post("/v1/artifacts", { ...payload, project_id: projectId || undefined });
      if (opts.moveSecretsToVault) notify("敏感内容已改存凭证库，正文已脱敏");
      await onDone();
    } catch (e) {
      const err = e as Error & { status?: number };
      if (err.status === 409) {
        const move = confirm(`${err.message}\n\n改存凭证库并脱敏保存？\n（取消则询问是否仍以明文保存）`);
        if (move) return save({ moveSecretsToVault: true });
        if (confirm("仍以明文保存到成稿库？")) return save({ acknowledgeSecret: true });
      }
      notify(err.message, "error");
    }
  }

  return (
    <form className="stack-tight" onSubmit={(e) => { e.preventDefault(); void save(); }}>
      <div className="row">
        <input className="wide" placeholder="标题" value={title} onChange={(e) => setTitle(e.target.value)} />
        <select value={type} onChange={(e) => setType(e.target.value)}>
          {Object.entries(TYPES).filter(([k]) => k).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
        <input placeholder="标签，逗号分隔" value={tags} onChange={(e) => setTags(e.target.value)} />
      </div>
      <textarea rows={14} value={body} onChange={(e) => setBody(e.target.value)} placeholder="Markdown 正文" required />
      <div className="row"><button className="primary">{initial ? "保存为新版本" : "保存"}</button></div>
    </form>
  );
}
