import { useState } from "react";
import { fmtTime, useApp } from "../ctx";
import { Empty, Section } from "../ui";

export default function ProjectsPage() {
  const { api, projects, refreshProjects, projectId, setProjectId, notify } = useApp();
  const [name, setName] = useState("");

  async function create() {
    try {
      await api.post("/v1/projects", { name });
      setName("");
      await refreshProjects();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function remove(id: string, pname: string) {
    const typed = prompt(`删除项目会同时删除它的会话、原文、附件、记忆、凭证和成稿，且不可恢复。\n输入项目名「${pname}」确认：`);
    if (typed !== pname) return;
    try {
      await api.del(`/v1/projects/${id}`);
      if (projectId === id) setProjectId("");
      notify("项目已删除");
      await refreshProjects();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  return (
    <Section title="项目" actions={<span className="muted small">项目记忆只在该项目内召回；偏好等全局记忆跨项目共享</span>}>
      <form className="row" onSubmit={(e) => { e.preventDefault(); void create(); }}>
        <input className="wide" value={name} onChange={(e) => setName(e.target.value)} placeholder="新项目名称" />
        <button className="primary" disabled={!name.trim()}>创建</button>
      </form>
      {projects.length === 0 && <Empty>还没有项目。未指定项目的会话归入全局。</Empty>}
      <table className="table">
        <thead><tr><th>名称</th><th>ID（配置到智能体的 project_id）</th><th>创建时间</th><th /></tr></thead>
        <tbody>
          {projects.map((p) => (
            <tr key={p.id}>
              <td><strong>{p.name}</strong></td>
              <td><code>{p.id}</code></td>
              <td className="muted">{fmtTime(p.created_at)}</td>
              <td><button className="small danger" onClick={() => void remove(p.id, p.name)}>删除</button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </Section>
  );
}
