import { useState } from "react";
import { API_BASE, type APIKeyInfo, type RecallReceipt } from "../api/client";
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
  const receipts = useLoad(() => api.get<RecallReceipt[]>("/v1/recall/receipts", { limit: 10 }), [api]);
  const stats = useLoad(() => api.get<{ by_agent: { agent: string; messages: number; unknown_model_pct: number; empty_skills_pct: number }[] }>("/v1/provenance/stats"), [api]);
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

      <Section title="最近召回（Receipt）" actions={<span className="muted small">用于确认本会话是否已注入记忆</span>}>
        <ErrorLine error={receipts.error} />
        {receipts.data?.length === 0 && <Empty>暂无召回记录。</Empty>}
        {!!receipts.data?.length && (
          <table className="table">
            <thead><tr><th>时间</th><th>来源</th><th>命中</th><th>状态</th></tr></thead>
            <tbody>
              {receipts.data.map((r) => (
                <tr key={r.id}>
                  <td className="muted">{fmtTime(r.queried_at)}</td>
                  <td>{r.source}{r.agent ? ` · ${r.agent}` : ""}</td>
                  <td>{r.hit_count}</td>
                  <td className="muted small">{r.degraded ? "降级" : "正常"}{r.warnings?.length ? ` · ${r.warnings.join(",")}` : ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title="溯源采集质量（近 7 天）">
        <ErrorLine error={stats.error} />
        {stats.data?.by_agent?.length === 0 && <Empty>暂无消息。</Empty>}
        {!!stats.data?.by_agent?.length && (
          <table className="table">
            <thead><tr><th>智能体</th><th>消息数</th><th>unknown model</th><th>空 skills</th></tr></thead>
            <tbody>
              {stats.data.by_agent.map((a) => (
                <tr key={a.agent || "(empty)"}>
                  <td>{a.agent || "—"}</td>
                  <td>{a.messages}</td>
                  <td>{Math.round(a.unknown_model_pct * 100)}%</td>
                  <td>{Math.round(a.empty_skills_pct * 100)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title="接入智能体（通用 MCP）">
        <p className="muted small">
          一键安装：<code>smctl install cursor|codex|hermes|openclaw|deepseek-harness|doubao</code>。
          Codex / DeepSeek / Hermes / OpenClaw 尽量自动注入；Cursor / 豆包用 MCP 召回（豆包另需导入 Skill）+ 上方 Receipt 可证明。
          通用 MCP 也可手写配置（SHARED_MEMORY_AGENT 填智能体名）。
        </p>
        <pre className="msg-body">{mcpConfig}</pre>
      </Section>
    </div>
  );
}
