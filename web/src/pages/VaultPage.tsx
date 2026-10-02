import { useState } from "react";
import type { VaultCandidate, VaultItem } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Chip, Empty, ErrorLine, Section } from "../ui";

const REVEAL_SECONDS = 30;

export default function VaultPage() {
  const { api, notify } = useApp();
  const candidates = useLoad(() => api.get<VaultCandidate[]>("/v1/vault/candidates"), [api]);
  const items = useLoad(() => api.get<VaultItem[]>("/v1/vault/items"), [api]);
  const [revealFor, setRevealFor] = useState<VaultItem | null>(null);
  const [password, setPassword] = useState("");
  const [shown, setShown] = useState<{ id: string; secret: string } | null>(null);
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState({ title: "", site_url: "", username: "", secret: "" });

  async function resolve(id: string, decision: "accept" | "reject") {
    try {
      await api.post(`/v1/vault/candidates/${id}/${decision}`);
      notify(decision === "accept" ? "已加密保存到凭证库" : "已丢弃，不会保存");
      await Promise.all([candidates.reload(), items.reload()]);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function reveal() {
    if (!revealFor) return;
    try {
      const res = await api.post<{ secret: string }>(`/v1/vault/items/${revealFor.id}/reveal`, { password });
      setShown({ id: revealFor.id, secret: res.secret });
      const id = revealFor.id;
      window.setTimeout(() => setShown((s) => (s?.id === id ? null : s)), REVEAL_SECONDS * 1000);
      setRevealFor(null);
      setPassword("");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function add() {
    try {
      await api.post("/v1/vault/items", draft);
      setDraft({ title: "", site_url: "", username: "", secret: "" });
      setAdding(false);
      await items.reload();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function remove(it: VaultItem) {
    if (!confirm(`删除「${it.title}」的凭证？`)) return;
    try {
      await api.del(`/v1/vault/items/${it.id}`);
      await items.reload();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  return (
    <div className="stack">
      <Section title="待确认" actions={<span className="muted small">智能体在对话中检测到的账号口令，需你同意才会加密保存（7 天后自动丢弃）</span>}>
        <ErrorLine error={candidates.error} />
        {candidates.data?.length === 0 && <Empty>没有待确认的凭证。</Empty>}
        <ul className="list">
          {candidates.data?.map((c) => (
            <li key={c.id} className="item static">
              <div className="item-meta">
                <strong>{c.title}</strong>
                <span>{c.site_url}</span>
                <span className="muted">账号 {c.username || "—"}</span>
                <Chip tone="agent">{c.agent_path}</Chip>
                <span className="spacer" />
                <button className="primary small" onClick={() => void resolve(c.id, "accept")}>加密保存</button>
                <button className="small" onClick={() => void resolve(c.id, "reject")}>不保存</button>
              </div>
            </li>
          ))}
        </ul>
      </Section>

      <Section title="凭证库" actions={<button onClick={() => setAdding((v) => !v)}>{adding ? "取消" : "手动添加"}</button>}>
        {adding && (
          <form className="grid-form" onSubmit={(e) => { e.preventDefault(); void add(); }}>
            <input placeholder="名称" value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
            <input placeholder="网址" value={draft.site_url} onChange={(e) => setDraft({ ...draft, site_url: e.target.value })} />
            <input placeholder="账号" value={draft.username} onChange={(e) => setDraft({ ...draft, username: e.target.value })} />
            <input placeholder="密码 / 口令" type="password" value={draft.secret} onChange={(e) => setDraft({ ...draft, secret: e.target.value })} required />
            <button className="primary">保存</button>
          </form>
        )}
        <ErrorLine error={items.error} />
        {items.data?.length === 0 && <Empty>凭证库为空。</Empty>}
        <table className="table">
          <thead>
            <tr><th>名称</th><th>网址</th><th>账号</th><th>口令</th><th>来源</th><th>更新</th><th /></tr>
          </thead>
          <tbody>
            {items.data?.map((it) => (
              <tr key={it.id}>
                <td>{it.title}</td>
                <td>{it.site_url ? <a href={it.site_url} target="_blank" rel="noreferrer">{it.site_url}</a> : "—"}</td>
                <td>{it.username || "—"}</td>
                <td>
                  {shown?.id === it.id ? (
                    <span className="row">
                      <code className="secret">{shown.secret}</code>
                      <button className="small" onClick={() => void navigator.clipboard.writeText(shown.secret)}>复制</button>
                    </span>
                  ) : (
                    <button className="small" onClick={() => setRevealFor(it)}>查看</button>
                  )}
                </td>
                <td><Chip tone="agent">{it.source_agent_path || (it.consent_method === "manual" ? "手动" : "—")}</Chip></td>
                <td className="muted">{fmtTime(it.updated_at)}</td>
                <td><button className="small danger" onClick={() => void remove(it)}>删除</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>

      {revealFor && (
        <div className="modal-backdrop" onClick={() => setRevealFor(null)}>
          <form className="modal" onClick={(e) => e.stopPropagation()} onSubmit={(e) => { e.preventDefault(); void reveal(); }}>
            <h3>查看「{revealFor.title}」的口令</h3>
            <p className="muted small">为安全起见需要再次输入账号密码；每次查看都会记录审计日志，口令 {REVEAL_SECONDS} 秒后自动隐藏。</p>
            <input type="password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} placeholder="账号密码" />
            <div className="row">
              <button className="primary" disabled={!password}>确认查看</button>
              <button type="button" onClick={() => setRevealFor(null)}>取消</button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
