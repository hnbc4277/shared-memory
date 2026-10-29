import { useEffect, useMemo, useState } from "react";
import { subscribeAgentsStream } from "../agentsStream";
import { useApp, useLoad, fmtTime } from "../ctx";
import { DOC_LINKS } from "../docLinks";
import type { RecallReceipt } from "../api/client";
import { ConfirmModal, Empty, ErrorLine, Modal, PageStack, Section, StatRow, StatTile } from "../ui";
import { CursorSetupGuide } from "./CursorSetupGuide";
import { DoubaoSetupGuide } from "./DoubaoSetupGuide";

interface Connection {
  id: string;
  agent: string;
  label: string;
  kind: string;
  process_online?: boolean;
  activity_fresh?: boolean;
  online: boolean;
  last_ingest_at?: string | null;
  last_recall_at?: string | null;
  last_heartbeat_at?: string | null;
  last_agent_path?: string;
  last_model?: string;
  last_external_session_id?: string;
  client_name?: string;
  daemon_version?: string;
  spool_pending?: number;
  api_key_name?: string;
  has_setup_script?: boolean;
  created_at?: string;
}

function truncId(s: string | undefined, max = 28): string {
  if (!s) return "";
  return s.length <= max ? s : `${s.slice(0, max)}…`;
}

function formatActivityAge(ingest?: string | null, recall?: string | null): string {
  const times = [ingest, recall].filter(Boolean).map((s) => new Date(s!).getTime());
  if (!times.length) return "尚无活动";
  const ageMs = Date.now() - Math.max(...times);
  if (ageMs < 60_000) return "刚刚";
  const mins = Math.floor(ageMs / 60_000);
  if (mins < 60) return `${mins} 分钟前`;
  const hours = Math.floor(mins / 60);
  if (hours < 48) return `${hours} 小时前`;
  return `${Math.floor(hours / 24)} 天前`;
}

/** Tag tone for activity age label from formatActivityAge. */
function activityTagClass(ageLabel: string): string {
  if (ageLabel === "尚无活动") return "tag is-muted";
  if (ageLabel === "刚刚" || ageLabel.endsWith("分钟前")) return "tag is-ok";
  if (ageLabel.endsWith("小时前")) return "tag is-amber";
  return "tag is-muted";
}

interface AgentGroup {
  agent: string;
  display_name: string;
  instance_count: number;
  online_count: number;
  last_activity_at?: string | null;
  instances: Connection[];
}

const KIND: Record<string, string> = {
  auto: "自动（API 密钥）",
  registered: "已注册连接",
  device_linked: "设备 / 守护进程",
};

const AGENT_OPTIONS = [
  { id: "cursor", label: "Cursor" },
  { id: "codex", label: "Codex" },
  { id: "xiaolongxia", label: "小龙虾" },
  { id: "hermes", label: "爱马仕" },
  { id: "deepseek-harness", label: "DeepSeek Harness" },
  { id: "openclaw", label: "OpenClaw" },
  { id: "doubao", label: "豆包" },
];

export default function AgentsPage() {
  const { api, notify, navigateTab } = useApp();
  const [streamGroups, setStreamGroups] = useState<AgentGroup[] | null>(null);
  const list = useLoad(() => api.get<{ groups: AgentGroup[] }>("/v1/agents"), [api]);
  /** SSE 缓存优先于 list；变更后必须同步，否则 reload 了界面仍显示旧流。 */
  async function refreshAgents() {
    const data = await list.reload();
    setStreamGroups(data?.groups ?? null);
  }

  useEffect(() => {
    return subscribeAgentsStream(
      (payload) => setStreamGroups(payload.groups as AgentGroup[]),
      () => {
        void list.reload().then((data) => {
          setStreamGroups(data?.groups ?? null);
        });
      },
    );
  }, [list.reload]);
  const groups = streamGroups ?? list.data?.groups;

  useEffect(() => {
    const id = sessionStorage.getItem("sm.agentsConnectionId");
    if (!id) return;
    sessionStorage.removeItem("sm.agentsConnectionId");
    window.setTimeout(() => {
      document.getElementById(`agent-conn-${id}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
    }, 200);
  }, [groups]);
  const stats = useMemo(() => {
    let online = 0;
    let total = 0;
    for (const g of groups ?? []) {
      online += g.online_count;
      total += g.instance_count;
    }
    return {
      online,
      total,
      offline: Math.max(0, total - online),
      products: groups?.length ?? 0,
    };
  }, [groups]);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [showAdd, setShowAdd] = useState(false);
  const [agent, setAgent] = useState("cursor");
  const [label, setLabel] = useState("");
  const [issueKey, setIssueKey] = useState(false);
  const [createClaimHistory, setCreateClaimHistory] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<Connection | null>(null);
  const [revokeAlsoKey, setRevokeAlsoKey] = useState(false);
  const [receiptConn, setReceiptConn] = useState<string | null>(null);
  const [receiptRows, setReceiptRows] = useState<RecallReceipt[] | null>(null);
  const [issued, setIssued] = useState<{ connection_id: string; api_key?: string; agent?: string } | null>(null);
  const [setupGuide, setSetupGuide] = useState<{ agent: string; connectionId: string; apiKey?: string } | null>(null);
  const [editId, setEditId] = useState<string | null>(null);
  const [editLabel, setEditLabel] = useState("");
  const [claimTarget, setClaimTarget] = useState<Connection | null>(null);
  const [claimUseBefore, setClaimUseBefore] = useState(false);
  const [claimBeforeDate, setClaimBeforeDate] = useState("");
  const [upgradeTarget, setUpgradeTarget] = useState<{ id: string; defaultLabel: string; agent: string } | null>(null);
  const [upgradeLabel, setUpgradeLabel] = useState("");
  const [upgradeIssueKey, setUpgradeIssueKey] = useState(false);
  const [upgradeClaimHistory, setUpgradeClaimHistory] = useState(false);
  const [downloadTarget, setDownloadTarget] = useState<Connection | null>(null);

  async function saveLabel(id: string) {
    try {
      await api.patch(`/v1/agents/connections/${id}`, { label: editLabel });
      setEditId(null);
      await refreshAgents();
      notify("备注已保存");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function addConnection() {
    try {
      const res = await api.post<{ connection_id: string; api_key?: string }>("/v1/agents/connections", {
        agent,
        label: label || undefined,
        issue_dedicated_key: issueKey,
        claim_history: createClaimHistory,
      });
      setIssued({ connection_id: res.connection_id, api_key: res.api_key, agent });
      if (agent === "cursor" || agent === "doubao") {
        setSetupGuide({ agent, connectionId: res.connection_id, apiKey: res.api_key });
      }
      setShowAdd(false);
      setLabel("");
      setCreateClaimHistory(false);
      await refreshAgents();
      notify(createClaimHistory ? "连接已创建（已认领同产品未绑定会话）" : "连接已创建");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  function openUpgrade(c: Connection) {
    if (c.kind !== "auto") return;
    setClaimTarget(null);
    setUpgradeTarget({ id: c.id, defaultLabel: c.label, agent: c.agent });
    setUpgradeLabel(c.label.replace(/^未绑定.*/, "我的连接") || "我的连接");
    setUpgradeIssueKey(false);
    setUpgradeClaimHistory(false);
  }

  async function submitUpgrade() {
    const u = upgradeTarget;
    if (!u) return;
    try {
      const res = await api.post<{ connection_id: string; api_key?: string }>(
        `/v1/agents/connections/${u.id}/upgrade`,
        {
          label: upgradeLabel.trim() || undefined,
          issue_dedicated_key: upgradeIssueKey,
          claim_history: upgradeClaimHistory,
        },
      );
      setUpgradeTarget(null);
      setIssued({ connection_id: res.connection_id, api_key: res.api_key, agent: u.agent });
      if (u.agent === "cursor" || u.agent === "doubao") {
        setSetupGuide({ agent: u.agent, connectionId: res.connection_id, apiKey: res.api_key });
      }
      await refreshAgents();
      notify(
        upgradeClaimHistory
          ? "已升级为正式连接（若勾选认领，未绑定会话将归属到新连接）"
          : "已升级为正式连接",
      );
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  function openClaim(c: Connection) {
    if (c.kind !== "registered") return;
    setUpgradeTarget(null);
    setClaimTarget(c);
    setClaimUseBefore(false);
    setClaimBeforeDate("");
  }

  async function submitClaim() {
    const c = claimTarget;
    if (!c) return;
    const body: { before?: string } = {};
    if (claimUseBefore) {
      const trimmed = claimBeforeDate.trim();
      if (!trimmed) {
        notify("请选择截止日期，或取消「仅认领此前会话」", "error");
        return;
      }
      const d = new Date(`${trimmed}T23:59:59.000Z`);
      if (Number.isNaN(d.getTime())) {
        notify("日期无效", "error");
        return;
      }
      body.before = d.toISOString();
    }
    try {
      const res = await api.post<{ sessions_claimed: number }>(
        `/v1/agents/connections/${c.id}/claim-history`,
        body,
      );
      setClaimTarget(null);
      notify(`已认领 ${res.sessions_claimed} 个会话（后台会更新索引中的相关 memory）`);
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  function openRevoke(c: Connection) {
    setRevokeTarget(c);
    setRevokeAlsoKey(false);
  }

  async function submitRevoke() {
    const c = revokeTarget;
    if (!c) return;
    let path = `/v1/agents/connections/${c.id}`;
    if (revokeAlsoKey && c.api_key_name) path += "?revoke_api_key=true";
    try {
      await api.del(path);
      setRevokeTarget(null);
      setDownloadTarget((t) => (t?.id === c.id ? null : t));
      setSetupGuide((g) => (g?.connectionId === c.id ? null : g));
      await refreshAgents();
      notify("已吊销");
    } catch (e) {
      notify((e as Error).message, "error");
    }
  }

  async function downloadSetupScript(c: Connection) {
    try {
      const blob = await api.getBlob(`/v1/agents/connections/${c.id}/setup-script`);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `setup-${c.agent}-${c.id.replace(/-/g, "").slice(0, 12)}.zip`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      setDownloadTarget(null);
      notify("已下载配置包：解压后双击 .cmd 即可，脚本已含本连接密钥与 connection_id");
    } catch (e) {
      notify((e as Error).message || "无法下载配置脚本", "error");
    }
  }

  async function toggleReceipt(c: Connection) {
    if (receiptConn === c.id) {
      setReceiptConn(null);
      setReceiptRows(null);
      return;
    }
    setReceiptConn(c.id);
    try {
      const rows = await api.get<RecallReceipt[]>("/v1/recall/receipts", { connection_id: c.id, limit: 3 });
      setReceiptRows(rows);
    } catch (e) {
      notify((e as Error).message, "error");
      setReceiptConn(null);
      setReceiptRows(null);
    }
  }

  function lastActivity(c: Connection): string {
    const times = [c.last_heartbeat_at, c.last_ingest_at, c.last_recall_at].filter(Boolean) as string[];
    if (times.length === 0) return "—";
    return fmtTime(times.sort().reverse()[0]);
  }

  return (
    <PageStack>
      <StatRow>
        <StatTile label="在线连接" value={`${stats.online}/${stats.total}`} tone={stats.offline > 0 ? "warn" : "ok"} />
        <StatTile label="离线" value={stats.offline} tone={stats.offline > 0 ? "warn" : "neutral"} />
        <StatTile label="产品分组" value={stats.products} hint="Cursor、Codex 等" />
        <StatTile
          label="数据"
          value="实时"
          hint="与顶栏概览条同步推送"
          tone="primary"
        />
      </StatRow>

      <Section
        title="连接实例"
        actions={
          <div className="row">
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => {
                setShowAdd(true);
                setIssued(null);
                setSetupGuide(null);
              }}
            >
              添加连接
            </button>
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() => {
                sessionStorage.removeItem("sm.historyAgent");
                sessionStorage.removeItem("sm.historyConnectionId");
                navigateTab("history");
              }}
            >
              查看全部历史会话
            </button>
          </div>
        }
      >
        <p className="lede muted small">
          按产品分组查看连接实例。仅用 API 密钥时会出现「自动」行；多台设备请添加连接并在客户端配置{" "}
          <code>SHARED_MEMORY_CONNECTION_ID</code> 或 ingest/recall 的 <code>connection_id</code>。
          创建连接时勾选「专用 API 密钥」后，用该密钥请求可<strong>自动</strong>归属到本连接，无需再填 <code>connection_id</code>。
          安装：<code>smctl install cursor</code>（
          <a href={DOC_LINKS.installCursor} target="_blank" rel="noreferrer">设计说明</a>
          ）/ hermes / openclaw / deepseek-harness / doubao；
          适配器文档：
          <a href={DOC_LINKS.hermes} target="_blank" rel="noreferrer">Hermes</a>
          、
          <a href={DOC_LINKS.openclaw} target="_blank" rel="noreferrer">OpenClaw</a>
          、
          <a href={DOC_LINKS.deepseek} target="_blank" rel="noreferrer">DeepSeek</a>
          、
          <a href={DOC_LINKS.doubao} target="_blank" rel="noreferrer">豆包</a>
          （
          <a href={DOC_LINKS.agentsSpec} target="_blank" rel="noreferrer">连接规格</a>
          ）。
        </p>
        <ErrorLine error={list.error} />
        {list.loading && <p className="muted">加载中…</p>}
        {!list.loading && (groups?.length ?? 0) === 0 && (
          <Empty>尚无活动。配置 API 密钥并完成一次上报后会出现自动连接。</Empty>
        )}
        {groups?.map((g) => {
          const expanded = open[g.agent] ?? true;
          return (
            <div key={g.agent} className="agent-group agent-group-cards">
              <button
                type="button"
                className="agent-group-head"
                onClick={() => setOpen((o) => ({ ...o, [g.agent]: !expanded }))}
              >
                <span className="agent-group-title">
                  <strong>{g.display_name}</strong>
                  <span className="muted small">/{g.agent}</span>
                </span>
                <span className="row muted small">
                  <span className="tnum">在线 {g.online_count}/{g.instance_count}</span>
                  {g.last_activity_at ? <span>· 最近 {fmtTime(g.last_activity_at)}</span> : null}
                  <button
                    type="button"
                    className="btn link"
                    onClick={(e) => {
                      e.stopPropagation();
                      navigateTab("history", { historyAgent: g.agent });
                    }}
                  >
                    查看历史
                  </button>
                </span>
              </button>
              {expanded && (
                <div className="conn-card-grid">
                  {g.instances.map((c) => {
                    const online = c.process_online ?? false;
                    return (
                      <article
                        key={c.id}
                        id={`agent-conn-${c.id}`}
                        className={`conn-card m-surface${online ? " is-online" : ""}`}
                      >
                        <header className="conn-card-top">
                          <span className={`tag${online ? " is-ok" : " is-danger"}`}>
                            <span className={online ? "dot-online" : "dot"} aria-hidden />
                            {online ? "进程在线" : "进程离线"}
                          </span>
                          <span className="tag is-violet">{KIND[c.kind] ?? c.kind}</span>
                        </header>

                        <div className="conn-card-main">
                          <div className="conn-card-title-row">
                            <h3 className="conn-card-title">{c.label || "未命名连接"}</h3>
                            <button
                              type="button"
                              className="btn link"
                              onClick={() => {
                                setEditId(c.id);
                                setEditLabel(c.label);
                              }}
                            >
                              编辑
                            </button>
                          </div>

                          <div className="conn-card-sub">
                            {(() => {
                              const age = formatActivityAge(c.last_ingest_at, c.last_recall_at);
                              return <span className={activityTagClass(age)}>{`活跃 · ${age}`}</span>;
                            })()}
                            <span className="muted small">最近 {lastActivity(c)}</span>
                          </div>

                          {c.kind === "auto" && (
                            <p className="conn-hint small">提示：多机请「添加连接」并配置 connection_id</p>
                          )}
                          {g.agent === "doubao" &&
                            c.process_online &&
                            !c.last_ingest_at &&
                            !c.last_recall_at && (
                              <p className="conn-hint small">
                                进程在线仅表示 MCP 心跳正常；须在豆包<strong>工作模式</strong>用 Skill 调用{" "}
                                <code>memory_ingest</code> 才会出现历史会话。
                              </p>
                            )}

                          {receiptConn === c.id && receiptRows && (
                            <div className="receipt-inline">
                              {receiptRows.length === 0 && <div className="muted small">暂无回执</div>}
                              {receiptRows.map((r) => (
                                <div key={r.id} className="small">
                                  {fmtTime(r.queried_at)} · 命中 {r.hit_count}
                                  {r.degraded ? " · 降级" : ""}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>

                        <dl className="conn-meta">
                          {c.api_key_name ? (
                            <>
                              <dt>Key</dt>
                              <dd>{c.api_key_name}</dd>
                            </>
                          ) : null}
                          {c.last_ingest_at ? (
                            <>
                              <dt>上报</dt>
                              <dd>{fmtTime(c.last_ingest_at)}</dd>
                            </>
                          ) : null}
                          {c.last_recall_at ? (
                            <>
                              <dt>召回</dt>
                              <dd>{fmtTime(c.last_recall_at)}</dd>
                            </>
                          ) : null}
                          {c.last_heartbeat_at ? (
                            <>
                              <dt>心跳</dt>
                              <dd>{fmtTime(c.last_heartbeat_at)}</dd>
                            </>
                          ) : null}
                          {c.last_agent_path ? (
                            <>
                              <dt>路径</dt>
                              <dd className="mono">{c.last_agent_path}</dd>
                            </>
                          ) : null}
                          {c.last_model ? (
                            <>
                              <dt>模型</dt>
                              <dd>{c.last_model}</dd>
                            </>
                          ) : null}
                          {c.daemon_version ? (
                            <>
                              <dt>守护进程</dt>
                              <dd>{c.daemon_version}</dd>
                            </>
                          ) : null}
                          {c.spool_pending != null && c.spool_pending > 0 ? (
                            <>
                              <dt>积压</dt>
                              <dd>{c.spool_pending}</dd>
                            </>
                          ) : null}
                          {c.last_external_session_id ? (
                            <>
                              <dt>外部会话</dt>
                              <dd title={c.last_external_session_id}>{truncId(c.last_external_session_id)}</dd>
                            </>
                          ) : null}
                          {c.created_at ? (
                            <>
                              <dt>创建</dt>
                              <dd>{fmtTime(c.created_at)}</dd>
                            </>
                          ) : null}
                          {!c.api_key_name &&
                            !c.last_ingest_at &&
                            !c.last_recall_at &&
                            !c.last_heartbeat_at &&
                            !c.last_agent_path &&
                            !c.last_model &&
                            !c.daemon_version &&
                            !(c.spool_pending != null && c.spool_pending > 0) &&
                            !c.last_external_session_id &&
                            !c.created_at && (
                              <>
                                <dt>详情</dt>
                                <dd className="muted">尚无更多元数据</dd>
                              </>
                            )}
                        </dl>

                        <footer className="conn-card-actions">
                          {c.kind === "auto" && (
                            <button type="button" className="btn btn-primary small" onClick={() => openUpgrade(c)}>
                              升级为正式连接
                            </button>
                          )}

                          {c.kind === "registered" && (
                            <>
                              <button
                                type="button"
                                className="btn btn-primary small"
                                onClick={() =>
                                  navigateTab("history", { historyAgent: g.agent, historyConnectionId: c.id })
                                }
                              >
                                查看历史会话
                              </button>
                              <button type="button" className="btn btn-quiet small" onClick={() => void toggleReceipt(c)}>
                                {receiptConn === c.id ? "收起回执" : "查看回执"}
                              </button>
                              {c.has_setup_script && (
                                <button
                                  type="button"
                                  className="btn btn-quiet small"
                                  onClick={() => {
                                    setRevokeTarget(null);
                                    setDownloadTarget(c);
                                  }}
                                >
                                  下载配置脚本
                                </button>
                              )}
                              {(g.agent === "cursor" || g.agent === "doubao") && (
                                <button
                                  type="button"
                                  className="btn btn-quiet small"
                                  onClick={() => setSetupGuide({ agent: g.agent, connectionId: c.id })}
                                >
                                  配置步骤
                                </button>
                              )}
                              <button
                                type="button"
                                className="btn btn-quiet small"
                                onClick={() => navigateTab("keys", { receiptConnectionId: c.id })}
                              >
                                密钥页
                              </button>
                              <button type="button" className="btn btn-quiet small" onClick={() => openClaim(c)}>
                                认领历史
                              </button>
                              <button type="button" className="btn btn-quiet small danger" onClick={() => openRevoke(c)}>
                                吊销
                              </button>
                            </>
                          )}

                          {c.kind === "device_linked" && (
                            <button
                              type="button"
                              className="btn btn-primary small"
                              onClick={() => {
                                sessionStorage.setItem("sm.scrollToDevices", "1");
                                navigateTab("keys");
                              }}
                            >
                              密钥页管理
                            </button>
                          )}
                        </footer>
                      </article>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </Section>

      {showAdd && (
        <Modal
          title="添加正式连接"
          onClose={() => setShowAdd(false)}
          footer={
            <div className="row">
              <button type="button" className="btn btn-primary" onClick={() => void addConnection()}>创建</button>
              <button type="button" className="btn btn-quiet" onClick={() => setShowAdd(false)}>取消</button>
            </div>
          }
        >
          <label className="field">
            <span>产品</span>
            <select value={agent} onChange={(e) => setAgent(e.target.value)}>
              {AGENT_OPTIONS.map((o) => (
                <option key={o.id} value={o.id}>{o.label}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>备注</span>
            <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="例如：公司 PC" />
          </label>
          <label className="row toggle-inline">
            <input type="checkbox" checked={issueKey} onChange={(e) => setIssueKey(e.target.checked)} />
            <span>同时签发专用 API 密钥</span>
          </label>
          <label className="row toggle-inline">
            <input type="checkbox" checked={createClaimHistory} onChange={(e) => setCreateClaimHistory(e.target.checked)} />
            <span>将此前未绑定 connection 的同产品会话归属到新连接</span>
          </label>
        </Modal>
      )}

      {editId && (
        <Modal
          title="编辑备注"
          onClose={() => setEditId(null)}
          footer={
            <div className="row">
              <button type="button" className="btn btn-primary" onClick={() => void saveLabel(editId)}>保存</button>
              <button type="button" className="btn btn-quiet" onClick={() => setEditId(null)}>取消</button>
            </div>
          }
        >
          <label className="field">
            <span>备注</span>
            <input value={editLabel} onChange={(e) => setEditLabel(e.target.value)} autoFocus />
          </label>
        </Modal>
      )}

      {issued ? (
        <Modal
          title="新连接凭证"
          onClose={() => setIssued(null)}
          wide
          footer={
            (issued.agent === "cursor" || issued.agent === "doubao") && setupGuide ? (
              <div className="row">
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={() => setIssued(null)}
                >
                  下一步：配置步骤
                </button>
              </div>
            ) : undefined
          }
        >
          <p className="small muted">
            {issued.agent === "cursor"
              ? "Cursor 请按配置步骤保存密钥、下载程序并写入钩子。密钥只显示这一次。"
              : issued.agent === "doubao"
                ? "豆包请按配置步骤填写 STDIO 连接器与环境变量。密钥只显示这一次。"
                : "将 connection_id 写入 ingest / recall JSON，或在客户端配置环境变量。"}
          </p>
          <div className="code-block">
            <span className="code-block-label">connection_id</span>
            <code>{issued.connection_id}</code>
          </div>
          {issued.api_key && (
            <div className="code-block">
              <span className="code-block-label">专用 API 密钥（仅显示一次）</span>
              <code className="secret inline-secret">{issued.api_key}</code>
            </div>
          )}
        </Modal>
      ) : setupGuide?.agent === "cursor" ? (
        <CursorSetupGuide
          connectionId={setupGuide.connectionId}
          apiKey={setupGuide.apiKey}
          onClose={() => setSetupGuide(null)}
        />
      ) : setupGuide?.agent === "doubao" ? (
        <DoubaoSetupGuide
          connectionId={setupGuide.connectionId}
          apiKey={setupGuide.apiKey}
          onClose={() => setSetupGuide(null)}
        />
      ) : null}

      {upgradeTarget && (
        <Modal
          title="升级为正式连接"
          onClose={() => setUpgradeTarget(null)}
          footer={
            <div className="row">
              <button type="button" className="btn btn-primary" onClick={() => void submitUpgrade()}>确认升级</button>
              <button type="button" className="btn btn-quiet" onClick={() => setUpgradeTarget(null)}>取消</button>
            </div>
          }
        >
          <p className="small muted">
            将「<strong>{upgradeTarget.agent}</strong>」的自动连接升级为正式连接，便于多机区分与专用 Key 归属。
          </p>
          <label className="field">
            <span>备注</span>
            <input value={upgradeLabel} onChange={(e) => setUpgradeLabel(e.target.value)} placeholder="例如：公司 PC" />
          </label>
          <label className="row toggle-inline">
            <input type="checkbox" checked={upgradeIssueKey} onChange={(e) => setUpgradeIssueKey(e.target.checked)} />
            <span>同时签发专用 API 密钥（仅显示一次）</span>
          </label>
          <label className="row toggle-inline">
            <input type="checkbox" checked={upgradeClaimHistory} onChange={(e) => setUpgradeClaimHistory(e.target.checked)} />
            <span>将此前未绑定 connection 的同产品会话归属到新连接</span>
          </label>
          <p className="small muted">认领按会话 <code>started_at</code>，不影响已绑定其他 connection 的会话。</p>
        </Modal>
      )}

      {downloadTarget && (
        <ConfirmModal
          title="下载配置包"
          danger
          message={
            <>
              即将下载「{downloadTarget.label || "未命名"}」的{" "}
              {AGENT_OPTIONS.find((o) => o.id === downloadTarget.agent)?.label || downloadTarget.agent}{" "}
              配置包。
              <br />
              <strong>包内脚本已写入该连接的 connection_id 与专用 API 密钥（明文）</strong>，
              执行时无需再填写；请勿转发、上传网盘或提交到公开仓库。
              <br />
              解压后双击 <code>.cmd</code> 即可安装。
            </>
          }
          confirmLabel="已知晓，下载"
          onConfirm={() => void downloadSetupScript(downloadTarget)}
          onCancel={() => setDownloadTarget(null)}
        />
      )}

      {revokeTarget && (
        <ConfirmModal
          title="吊销连接"
          message={<>吊销连接「{revokeTarget.label || revokeTarget.id}」？</>}
          confirmLabel="吊销"
          danger
          onConfirm={() => void submitRevoke()}
          onCancel={() => setRevokeTarget(null)}
        >
          {revokeTarget.api_key_name && (
            <label className="row toggle-inline">
              <input type="checkbox" checked={revokeAlsoKey} onChange={(e) => setRevokeAlsoKey(e.target.checked)} />
              <span>同时吊销专用 API 密钥「{revokeTarget.api_key_name}」</span>
            </label>
          )}
        </ConfirmModal>
      )}

      {claimTarget && (
        <Modal
          title="认领历史会话"
          onClose={() => setClaimTarget(null)}
          footer={
            <div className="row">
              <button type="button" className="btn btn-primary" onClick={() => void submitClaim()}>确认认领</button>
              <button type="button" className="btn btn-quiet" onClick={() => setClaimTarget(null)}>取消</button>
            </div>
          }
        >
          <p className="small muted">
            将未绑定 connection 的「<strong>{claimTarget.agent}</strong>」会话归属到「{claimTarget.label}」。
            按会话 <code>started_at</code> 筛选；已绑定其他 connection 的会话不受影响。
          </p>
          <label className="row toggle-inline">
            <input type="checkbox" checked={claimUseBefore} onChange={(e) => setClaimUseBefore(e.target.checked)} />
            <span>仅认领某一日期之前开始的会话（UTC 日末）</span>
          </label>
          {claimUseBefore && (
            <label className="field">
              <span>截止日期</span>
              <input type="date" value={claimBeforeDate} onChange={(e) => setClaimBeforeDate(e.target.value)} />
            </label>
          )}
        </Modal>
      )}
    </PageStack>
  );
}
