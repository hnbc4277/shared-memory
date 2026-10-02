import type { Policy } from "../api/client";
import { fmtTime, useApp, useLoad } from "../ctx";
import { Empty, ErrorLine, Section, Toggle } from "../ui";

const TOGGLES: { key: keyof Policy; label: string; hint: string }[] = [
  { key: "store_transcript", label: "保存聊天原文", hint: "关闭后只记录溯源信息（智能体、技能、插件、时间），不保存正文" },
  { key: "store_attachments", label: "保存附件文件", hint: "关闭后附件只保留文件名、类型、大小等元数据" },
  { key: "inject_enabled", label: "自动注入记忆", hint: "智能体开始任务时自动带上相关记忆" },
  { key: "vault_enabled", label: "检测账号口令", hint: "检测到口令时询问是否加密保存到凭证库（无论开关，口令都不会以明文存储）" },
  { key: "artifact_searchable", label: "成稿参与检索", hint: "允许召回时检索已保存的成稿内容" },
];

export default function SettingsPage() {
  const { api, notify, logout } = useApp();
  const policy = useLoad(() => api.get<Policy>("/v1/policy"), [api]);
  const audit = useLoad(() => api.get<{ id: string; action: string; meta: unknown; created_at: string }[]>("/v1/audit", { limit: 50 }), [api]);

  async function set(key: keyof Policy, value: boolean) {
    try {
      await api.put("/v1/policy", { [key]: value });
      await policy.reload();
      notify("设置已保存");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function deleteAccount() {
    const typed = prompt("这会永久删除账号下的全部会话、原文、附件、记忆、凭证、成稿和密钥。\n输入 DELETE 确认：");
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
        </div>
      </Section>

      <Section title="审计日志" actions={<span className="muted small">凭证查看、同意、删除等敏感操作</span>}>
        {audit.data?.length === 0 && <Empty>暂无记录。</Empty>}
        <table className="table">
          <tbody>
            {audit.data?.map((e) => (
              <tr key={e.id}>
                <td className="muted">{fmtTime(e.created_at)}</td>
                <td><code>{e.action}</code></td>
                <td className="muted small">{JSON.stringify(e.meta)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>

      <Section title="即将推出（M3）">
        <ul className="muted small">
          <li>设备码登录：在 CLI / 智能体中扫码或输入短码完成授权</li>
          <li>本地守护进程：自动采集各智能体对话并离线缓冲</li>
          <li>官方适配器：Cursor、Codex、爱马仕、小龙虾、DeepSeek-Harness</li>
        </ul>
      </Section>

      <Section title="危险操作">
        <button className="danger" onClick={() => void deleteAccount()}>永久删除账号及全部数据</button>
      </Section>
    </div>
  );
}
