import type { Policy } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Empty, ErrorLine, Section, Toggle } from "../ui";

function QuarantineList() {
  const { api, notify } = useApp();
  const list = useLoad(() => api.get<{ id: string; agent: string; reason: string; payload: unknown; created_at: string }[]>("/v1/quarantine", { limit: 50 }), [api]);

  async function replay(id: string) {
    try {
      await api.post(`/v1/quarantine/${id}/replay`, {});
      notify("重放成功，已移出隔离区");
      await list.reload();
    } catch (e) {
      notify((e as Error).message, "error");
      await list.reload();
    }
  }

  return (
    <>
      <ErrorLine error={list.error} />
      {list.data?.length === 0 && <Empty>暂无隔离记录。</Empty>}
      {!!list.data?.length && (
        <table className="table">
          <thead><tr><th>时间</th><th>智能体</th><th>原因</th><th /></tr></thead>
          <tbody>
            {list.data.map((e) => (
              <tr key={e.id}>
                <td className="muted">{fmtTime(e.created_at)}</td>
                <td>{e.agent || "—"}</td>
                <td className="small">{e.reason}</td>
                <td className="row-actions">
                  <button className="small" onClick={() => {
                    void navigator.clipboard.writeText(JSON.stringify(e.payload, null, 2));
                    notify("已复制 payload");
                  }}>复制 payload</button>
                  <button className="small" onClick={() => void replay(e.id)}>重放</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

const TOGGLES: { key: keyof Policy; label: string; hint: string }[] = [
  { key: "store_transcript", label: "保存聊天原文", hint: "关闭后只记录溯源信息（智能体、技能、插件、时间），不保存正文" },
  { key: "store_attachments", label: "保存附件文件", hint: "关闭后附件只保留文件名、类型、大小等元数据" },
  { key: "inject_enabled", label: "自动注入记忆", hint: "智能体开始任务时自动带上相关记忆" },
  { key: "kb_inject_enabled", label: "召回时注入知识库", hint: "memory_recall 时带上知识库片段（kb_hits）；关闭则只召回记忆" },
  { key: "vault_enabled", label: "检测账号口令", hint: "检测到口令时询问是否加密保存到凭证库（无论开关，口令都不会以明文存储）" },
  { key: "artifact_searchable", label: "成稿参与检索", hint: "允许召回时检索已保存的成稿内容" },
  { key: "cloud_vision_enabled", label: "云侧图片视觉描述", hint: "开启后，配置了 VISION_URL 的 Worker 可用多模态模型描述图片；关闭则仅用本地宽高/OCR" },
];

const RETENTION_OPTIONS = [0, 7, 30, 90, 180, 365];

export default function SettingsPage() {
  const { api, notify, logout } = useApp();
  const policy = useLoad(() => api.get<Policy>("/v1/policy"), [api]);
  const audit = useLoad(() => api.get<{ id: string; action: string; meta: unknown; created_at: string }[]>("/v1/audit", { limit: 50 }), [api]);

  async function set(key: keyof Policy, value: boolean | number) {
    try {
      await api.put("/v1/policy", { [key]: value });
      await Promise.all([policy.reload(), audit.reload()]);
      notify("设置已保存");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function deleteAccount() {
    const typed = prompt("这会永久删除账号下的全部会话、原文、附件、记忆、知识库、凭证、成稿和密钥。\n输入 DELETE 确认：");
    if (typed !== "DELETE") return;
    try {
      await api.del("/v1/account", { "X-Confirm": "delete" });
      logout();
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  return (
    <div className="stack">
      <Section title="存储与注入策略">
        <ErrorLine error={policy.error} />
        <div className="toggles">
          {policy.data && TOGGLES.map((t) => (
            <Toggle key={t.key} label={t.label} hint={t.hint} checked={Boolean(policy.data![t.key])} onChange={(v) => void set(t.key, v)} />
          ))}
          {policy.data && (
            <label className="toggle retention">
              <div>
                <strong>原文保留时长</strong>
                <small>超过时长的会话将删除原文、消息与附件；由其总结出的记忆、凭证和成稿会保留</small>
              </div>
              <select value={policy.data.retention_days} onChange={(e) => void set("retention_days", Number(e.target.value))}>
                {RETENTION_OPTIONS.map((d) => <option key={d} value={d}>{d === 0 ? "永久保留" : `${d} 天`}</option>)}
              </select>
            </label>
          )}
        </div>
      </Section>

      <Section title="隔离区（不合格上报）" actions={<span className="muted small">可「重放」；失败则更新原因并保留</span>}>
        <QuarantineList />
      </Section>

      <Section title="审计日志" actions={<span className="muted small">凭证查看、同意、删除等敏感操作</span>}>
        {audit.data?.length === 0 && <Empty>暂无记录。</Empty>}
        {!!audit.data?.length && <table className="table">
          <tbody>
            {audit.data?.map((e) => (
              <tr key={e.id}>
                <td className="muted">{fmtTime(e.created_at)}</td>
                <td><code>{e.action}</code></td>
                <td className="muted small">{JSON.stringify(e.meta)}</td>
              </tr>
            ))}
          </tbody>
        </table>}
      </Section>

      <Section title="设备码与守护进程">
        <ul className="muted small">
          <li>设备码登录：本机运行 <code>smctl login</code>，在「密钥 → 授权设备」确认用户码</li>
          <li>本地守护进程：<code>shared-memory-daemon</code> 负责离线缓冲与自动上报</li>
          <li>官方适配器：<code>smctl install cursor|codex|hermes|openclaw|deepseek-harness</code>（详见 <code>adapters/*/README.md</code>）</li>
        </ul>
      </Section>

      <Section title="危险操作">
        <button className="danger" onClick={() => void deleteAccount()}>永久删除账号及全部数据</button>
      </Section>
    </div>
  );
}
