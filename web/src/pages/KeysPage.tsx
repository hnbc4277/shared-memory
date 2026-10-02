import { useState } from "react";
import { API_BASE, type APIKeyInfo } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Empty, ErrorLine, Section } from "../ui";

interface Device {
  id: string;
  client_name: string;
  agent: string;
  last_seen_at: string | null;
  daemon_version?: string;
  spool_pending: number;
  created_at: string;
}

function stale(lastSeen: string | null): boolean {
  if (!lastSeen) return true;
  return Date.now() - new Date(lastSeen).getTime() > 5 * 60 * 1000;
}

export default function KeysPage() {
  const { api, notify } = useApp();
  const keys = useLoad(() => api.get<APIKeyInfo[]>("/v1/auth/api-keys"), [api]);
  const devices = useLoad(() => api.get<Device[]>("/v1/devices"), [api]);
  const [name, setName] = useState("");
  const [issued, setIssued] = useState<string | null>(null);

  async function create() {
    try {
      const res = await api.post<{ api_key: string }>("/v1/auth/api-keys", { name: name || "agent" });
      setIssued(res.api_key);
      setName("");
      await keys.reload();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function revoke(k: APIKeyInfo) {
    if (!confirm(`吊销「${k.name}」？使用它的智能体将无法再读写记忆。`)) return;
    try {
      await api.del(`/v1/auth/api-keys/${k.id}`);
      await keys.reload();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function revokeDevice(d: Device) {
    if (!confirm(`吊销设备「${d.client_name}」？其 API Key 会一并失效。`)) return;
    try {
      await api.del(`/v1/devices/${d.id}`);
      await Promise.all([devices.reload(), keys.reload()]);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  const mcpConfig = JSON.stringify(
    {
      mcpServers: {
        "shared-memory": {
          command: "shared-memory-mcp",
          env: { SHARED_MEMORY_API_URL: API_BASE, SHARED_MEMORY_API_KEY: issued ?? "<你的 API Key>", SHARED_MEMORY_AGENT: "cursor" },
        },
      },
    },
    null,
    2,
  );

  return (
    <div className="stack">
      <Section title="API Key" actions={<span className="muted small">建议每个智能体一把，方便单独吊销</span>}>
        <form className="row" onSubmit={(e) => { e.preventDefault(); void create(); }}>
          <input className="wide" value={name} onChange={(e) => setName(e.target.value)} placeholder="名称，例如 cursor-laptop、xiaolongxia" />
          <button className="primary">生成</button>
        </form>
        {issued && (
          <div className="issued">
            <p>新 Key <strong>只显示这一次</strong>：</p>
            <code className="secret">{issued}</code>
            <button className="small" onClick={() => void navigator.clipboard.writeText(issued)}>复制</button>
          </div>
        )}
        <table className="table">
          <thead><tr><th>名称</th><th>创建时间</th><th /></tr></thead>
          <tbody>
            {keys.data?.map((k) => (
              <tr key={k.id}>
                <td>{k.name}</td>
                <td className="muted">{fmtTime(k.created_at)}</td>
                <td><button className="small danger" onClick={() => void revoke(k)}>吊销</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>

      <Section title="已授权设备" actions={<button className="small" onClick={() => { location.hash = "device"; }}>授权新设备</button>}>
        <ErrorLine error={devices.error} />
        {devices.data?.length === 0 && <Empty>还没有通过设备码登录的设备。在本机运行 <code>smctl login</code>。</Empty>}
        {!!devices.data?.length && <table className="table">
          <thead>
            <tr><th>名称</th><th>智能体</th><th>Daemon</th><th>最后在线</th><th>积压</th><th /></tr>
          </thead>
          <tbody>
            {devices.data?.map((d) => (
              <tr key={d.id}>
                <td>{d.client_name}</td>
                <td className="muted">{d.agent || "—"}</td>
                <td>{d.daemon_version || "—"}{stale(d.last_seen_at) ? " · 离线" : " · 在线"}</td>
                <td className="muted">{d.last_seen_at ? fmtTime(d.last_seen_at) : "从未"}</td>
                <td>{d.spool_pending}</td>
                <td><button className="small danger" onClick={() => void revokeDevice(d)}>吊销</button></td>
              </tr>
            ))}
          </tbody>
        </table>}
      </Section>

      <Section title="接入智能体（通用 MCP）">
        <p className="muted small">任何支持 MCP 的智能体都可以用下面的配置接入；SHARED_MEMORY_AGENT 填写该智能体的名字（子智能体在调用时传 agent_path）。也可用 <code>smctl install cursor|codex</code> 一键写入 hooks。</p>
        <pre className="msg-body">{mcpConfig}</pre>
      </Section>
    </div>
  );
}
