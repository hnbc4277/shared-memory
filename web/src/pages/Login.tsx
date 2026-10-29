import { useEffect, useRef, useState, type FormEvent } from "react";
import { API_BASE, createClient } from "../api/client";
import { startNeural } from "../pearl/neuralCanvas";
import { syncSegmentThumb } from "../pearl/interactions";
import {
  bindLoginCardTilt,
  buildDialTicks,
  playLoginIntro,
  playSuccessOrb,
  tickDialClock,
} from "../pearl/motion";

type Mode = "key" | "login" | "register";

export default function Login({ onKey }: { onKey: (key: string) => void }) {
  const rootRef = useRef<HTMLDivElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const dialSvgRef = useRef<SVGSVGElement>(null);
  const dialTimeRef = useRef<HTMLDivElement>(null);
  const dialDateRef = useRef<HTMLDivElement>(null);
  const submitSlotRef = useRef<HTMLDivElement>(null);
  const submitBtnRef = useRef<HTMLButtonElement>(null);
  const neuralRef = useRef<ReturnType<typeof startNeural> | null>(null);

  const [mode, setMode] = useState<Mode>("key");
  const [key, setKey] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [healthMs, setHealthMs] = useState<string>("—");

  useEffect(() => {
    playLoginIntro(rootRef.current);
    const unbindTilt = bindLoginCardTilt(cardRef.current);
    buildDialTicks(dialSvgRef.current);
    tickDialClock(dialSvgRef.current, dialTimeRef.current, dialDateRef.current);
    const clock = window.setInterval(
      () => tickDialClock(dialSvgRef.current, dialTimeRef.current, dialDateRef.current),
      1000,
    );
    neuralRef.current = startNeural(canvasRef.current, "medium");
    return () => {
      window.clearInterval(clock);
      unbindTilt();
      neuralRef.current?.destroy();
      neuralRef.current = null;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const t0 = performance.now();
    void fetch(`${API_BASE.replace(/\/+$/, "")}/healthz`)
      .then((r) => {
        if (!cancelled && r.ok) setHealthMs(`${Math.round(performance.now() - t0)} ms`);
      })
      .catch(() => {
        if (!cancelled) setHealthMs("不可达");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const seg = rootRef.current?.querySelector<HTMLElement>(".login-mode-seg");
    const active = seg?.querySelector<HTMLElement>("button.is-active");
    if (seg && active) syncSegmentThumb(seg, active);
  }, [mode, issued]);

  async function enterWithKey(k: string) {
    setBusy(true);
    setError(null);
    try {
      await createClient(API_BASE, () => k).get("/v1/me");
      await playSuccessOrb(submitSlotRef.current, submitBtnRef.current);
      neuralRef.current?.burst();
      window.setTimeout(() => onKey(k), 280);
    } catch (e) {
      setError(`凭证无效：${(e as Error).message}`);
      setBusy(false);
    }
  }

  async function login() {
    setBusy(true);
    setError(null);
    try {
      const res = await createClient(API_BASE, () => null).post<{ token: string }>("/v1/auth/login", {
        email,
        password,
      });
      await createClient(API_BASE, () => res.token).get("/v1/me");
      await playSuccessOrb(submitSlotRef.current, submitBtnRef.current);
      neuralRef.current?.burst();
      window.setTimeout(() => onKey(res.token), 280);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  }

  async function register() {
    setBusy(true);
    setError(null);
    try {
      const res = await createClient(API_BASE, () => null).post<{ api_key: string }>("/v1/auth/register", {
        email,
        password,
      });
      setIssued(res.api_key);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (mode === "key") void enterWithKey(key.trim());
    else if (mode === "login") void login();
    else void register();
  }

  const canSubmit =
    !busy &&
    ((mode === "key" && key.trim().length > 0) ||
      (mode === "login" && email.length > 0 && password.length >= 8) ||
      (mode === "register" && !issued && email.length > 0 && password.length >= 8));

  return (
    <div className="login" id="login-root" ref={rootRef}>
      <section className="login-canvas" aria-label="产品介绍">
        <canvas id="neural-canvas" ref={canvasRef} aria-hidden="true" />
        <div className="canvas-grid" aria-hidden="true" />
        <div className="canvas-glow" aria-hidden="true" />

        <div className="login-brand">
          <span className="brand-mark">
            <span className="brand-dot" />
          </span>
          <div>
            <div className="brand-name">共享记忆</div>
            <div className="brand-sub">Shared Memory Console</div>
          </div>
        </div>

        <div className="canvas-main">
          <div className="canvas-copy">
            <span className="eyebrow hero-line">自托管 · 召回可审计</span>
            <h1 className="login-title">
              <span className="hero-line">跨智能体的一台</span>
              <span className="hero-line">长期记忆层。</span>
            </h1>
            <p className="login-lead hero-line">
              会话注入、结构化召回与回执、凭证与成稿库在同一控制台闭环；连接 Cursor、Codex、OpenClaw 等，不必各自记上下文。
            </p>
          </div>

          <div className="login-dial hero-line" aria-hidden="true">
            <svg viewBox="0 0 100 100" ref={dialSvgRef} />
            <div className="dial-center">
              <div className="dial-time" ref={dialTimeRef}>
                --:--:--
              </div>
              <div className="dial-date" ref={dialDateRef}>
                ----
              </div>
            </div>
          </div>
        </div>

        <div className="canvas-plates">
          <div className="login-plate is-health hero-line">
            <span className="eyebrow">服务健康</span>
            <span className="health-dot" />
            <span>API {healthMs === "不可达" ? "异常" : "就绪"}</span>
            <span style={{ fontFamily: "var(--font-mono)", color: "rgba(255,255,255,.7)" }}>{healthMs}</span>
            <span className="plate-sub">{API_BASE.replace(/^https?:\/\//, "")}</span>
          </div>
          <ul className="login-plate cap-list">
            <li className="cap-item hero-line">
              <span className="cap-ico">↻</span>
              <div>
                <span className="cap-title">会话注入</span>
                <span className="cap-desc">Hook / MCP 写入带 attribution</span>
              </div>
            </li>
            <li className="cap-item hero-line">
              <span className="cap-ico">◎</span>
              <div>
                <span className="cap-title">召回回执</span>
                <span className="cap-desc">每次 recall 可核对命中与策略</span>
              </div>
            </li>
            <li className="cap-item hero-line">
              <span className="cap-ico">⎔</span>
              <div>
                <span className="cap-title">智能体连接</span>
                <span className="cap-desc">在线心跳与 spool 状态一览</span>
              </div>
            </li>
            <li className="cap-item hero-line">
              <span className="cap-ico">⌁</span>
              <div>
                <span className="cap-title">凭证与成稿</span>
                <span className="cap-desc">密钥隔离、成稿库与知识库</span>
              </div>
            </li>
          </ul>
        </div>

        <p className="login-foot hero-line">PEARL GLASS · Shared Memory Console</p>
      </section>

      <section className="login-side">
        <div className="login-card m-float m-glow" id="login-card" ref={cardRef}>
          <div className="glow-layer" aria-hidden="true" />
          <div className="login-card-inner">
            <span className="eyebrow" style={{ color: "var(--ink-faint)" }}>
              控制台登录
            </span>
            <h2 className="login-card-title">{issued ? "密钥已签发" : "登录"}</h2>
            <p className="login-card-sub">
              {issued ? "请立即复制保存，关闭后无法再次查看完整密钥。" : "使用 API 密钥或邮箱账号进入控制台"}
            </p>

            {!issued && (
              <div className="segmented m-groove login-mode-seg" style={{ marginTop: 18 }}>
                <span className="seg-thumb" aria-hidden />
                <button type="button" className={mode === "key" ? "is-active" : ""} onClick={() => setMode("key")}>
                  API Key
                </button>
                <button type="button" className={mode === "login" ? "is-active" : ""} onClick={() => setMode("login")}>
                  邮箱登录
                </button>
                <button
                  type="button"
                  className={mode === "register" ? "is-active" : ""}
                  onClick={() => setMode("register")}
                >
                  注册
                </button>
              </div>
            )}

            {issued ? (
              <div className="issued" style={{ marginTop: 20 }}>
                <code className="secret">{issued}</code>
                <div className="row" style={{ marginTop: 12 }}>
                  <button type="button" className="btn btn-secondary" onClick={() => void navigator.clipboard.writeText(issued)}>
                    复制
                  </button>
                  <button type="button" className="btn btn-primary" onClick={() => onKey(issued)}>
                    已保存，进入控制台
                  </button>
                </div>
              </div>
            ) : (
              <form className="login-form" onSubmit={onSubmit} noValidate>
                {mode === "key" && (
                  <div>
                    <div className="field-label">API 密钥</div>
                    <div className="input-shell">
                      <input
                        type="password"
                        value={key}
                        onChange={(e) => setKey(e.target.value)}
                        placeholder="sm_…"
                        autoComplete="off"
                        autoFocus
                      />
                    </div>
                  </div>
                )}
                {(mode === "login" || mode === "register") && (
                  <>
                    <div>
                      <div className="field-label">邮箱</div>
                      <div className="input-shell">
                        <input
                          type="email"
                          value={email}
                          onChange={(e) => setEmail(e.target.value)}
                          required
                          autoFocus
                        />
                      </div>
                    </div>
                    <div>
                      <div className="field-label">密码{mode === "register" ? "（至少 8 位）" : ""}</div>
                      <div className="input-shell">
                        <input
                          type="password"
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                          minLength={8}
                          required
                        />
                      </div>
                    </div>
                  </>
                )}
                <div className="login-options">
                  <label>
                    <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} />{" "}
                    记住本机
                  </label>
                  {mode !== "register" ? (
                    <button type="button" className="btn link" onClick={() => setMode("register")}>
                      注册账号
                    </button>
                  ) : (
                    <button type="button" className="btn link" onClick={() => setMode("login")}>
                      已有账号
                    </button>
                  )}
                </div>
                <div className="login-submit-slot" ref={submitSlotRef}>
                  <button
                    type="submit"
                    className={`btn btn-primary login-submit${busy ? " is-loading" : ""}`}
                    id="login-submit-btn"
                    ref={submitBtnRef}
                    disabled={!canSubmit}
                  >
                    <span className="btn-label">
                      {mode === "register" ? "注册" : mode === "login" ? "登录" : "进入控制台"}
                    </span>
                    <span className="spinner" aria-hidden="true" />
                  </button>
                  <div className="success-orb" role="status" aria-label="登录成功，正在进入">
                    <svg width="30" height="30" viewBox="0 0 46 46" aria-hidden="true">
                      <circle cx="23" cy="23" r="20" />
                      <path d="m15 23.5 5.5 5.5L31.5 18" />
                    </svg>
                  </div>
                </div>
              </form>
            )}
            {error && <div className="error-line" style={{ marginTop: 12 }}>{error}</div>}
            <p className="muted small" style={{ marginTop: 14 }}>
              设备码登录用于本地守护进程与 CLI（smctl login）。
            </p>
          </div>
        </div>
      </section>
    </div>
  );
}
