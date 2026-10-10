import { useRef, useState } from "react";
import { API_BASE, buildUrl, type KBDocument } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Chip, Empty, ErrorLine, Section } from "../ui";

const SOURCE: Record<string, string> = { upload: "上传", artifact: "成稿", attachment: "附件" };
const STATUS: Record<string, string> = { pending: "解析中", ready: "就绪", failed: "失败" };

export default function KBPage() {
  const { api, notify } = useApp();
  const fileRef = useRef<HTMLInputElement>(null);
  const [title, setTitle] = useState("");
  const [openId, setOpenId] = useState<string | null>(null);
  const list = useLoad(() => api.get<{ documents: KBDocument[] }>("/v1/kb/documents", { limit: 100 }), [api]);

  async function upload(file: File) {
    const fd = new FormData();
    fd.append("file", file);
    if (title.trim()) fd.append("title", title.trim());
    const key = localStorage.getItem("sm.apiKey");
    const resp = await fetch(buildUrl(API_BASE, "/v1/kb/documents"), {
      method: "POST",
      headers: key ? { Authorization: `Bearer ${key}` } : {},
      body: fd,
    });
    const text = await resp.text();
    if (!resp.ok) {
      let msg = text || resp.statusText;
      try {
        msg = JSON.parse(text).error ?? msg;
      } catch {
        /* plain */
      }
      throw new Error(msg);
    }
    setTitle("");
    if (fileRef.current) fileRef.current.value = "";
    await list.reload();
    notify("已上传，正在解析");
  }

  async function remove(id: string) {
    if (!confirm("从知识库删除这篇文档及其索引？")) return;
    try {
      await api.del(`/v1/kb/documents/${id}`);
      if (openId === id) setOpenId(null);
      await list.reload();
      notify("已删除");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function reparse(id: string) {
    try {
      await api.post(`/v1/kb/documents/${id}/reparse`);
      await list.reload();
      notify("已重新入队解析");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  return (
    <div className="stack">
      <Section
        title="知识库"
        actions={
          <span className="muted small">账号全局资料；召回时经 memory_recall 注入（可在设置关闭）</span>
        }
      >
        <div className="row">
          <input placeholder="标题（可选）" value={title} onChange={(e) => setTitle(e.target.value)} />
          <input
            ref={fileRef}
            type="file"
            accept=".md,.markdown,.txt,.pdf,.docx,text/plain,text/markdown,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void upload(f).catch((err) => notify((err as Error).message, "error"));
            }}
          />
        </div>
        <ErrorLine error={list.error} />
        {list.data?.documents.length === 0 && <Empty>知识库为空。上传文档，或从成稿/附件「加入知识库」。</Empty>}
        <ul className="list">
          {list.data?.documents.map((d) => (
            <li
              key={d.id}
              className={openId === d.id ? "item selected" : "item"}
              onClick={() => setOpenId(openId === d.id ? null : d.id)}
            >
              <div className="item-meta">
                <strong>{d.title || "（无标题）"}</strong>
                <Chip>{SOURCE[d.source_type] ?? d.source_type}</Chip>
                <Chip tone={d.status === "ready" ? "skill" : d.status === "failed" ? "tool" : "agent"}>
                  {STATUS[d.status] ?? d.status}
                </Chip>
                <span className="muted small">{fmtTime(d.updated_at)}</span>
                <span className="spacer" />
                {(d.status === "failed" || d.status === "ready") && (
                  <button
                    className="small"
                    onClick={(e) => {
                      e.stopPropagation();
                      void reparse(d.id);
                    }}
                  >
                    重新解析
                  </button>
                )}
                <button
                  className="small danger"
                  onClick={(e) => {
                    e.stopPropagation();
                    void remove(d.id);
                  }}
                >
                  删除
                </button>
              </div>
              {openId === d.id && (
                <div className="viewer" onClick={(e) => e.stopPropagation()}>
                  <div className="muted small">MIME: {d.mime || "—"} · {d.bytes} 字节</div>
                  {d.error && <div className="error small">{d.error}</div>}
                </div>
              )}
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}
