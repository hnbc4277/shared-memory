import { useState } from "react";
import { API_BASE, createClient } from "../api/client";

export default function Login({ onKey }: { onKey: (key: string) => void }) {
  const [mode, setMode] = useState<"key" | "login" | "register">("key");
  const [key, setKey] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function tryKey(k: string) {
    setBusy(true);
    setError(null);
    try {
      await createClient(API_BASE, () => k).get("/v1/me");
      onKey(k);
    } catch (e) {
      setError(`凭证无效：${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  async function login() {
    setBusy(true);
    setError(null);
    try {
      const res = await createClient(API_BASE, () => null).post<{ token: string }>(
        "/v1/auth/login",
        { email, password },
      );
      await createClient(API_BASE, () => res.token).get("/v1/me");
      onKey(res.token);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function register() {
    setBusy(true);
    setError(null);
    try {
      const res = await createClient(API_BASE, () => null).post<{ api_key: string }>("/v1/auth/register", { email, password });
      setIssued(res.api_key);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <div className="login-card">
        <div className="brand">
          <span className="brand-mark" aria-hidden>◎</span>
          <div>
            <strong>共享记忆控制台</strong>
            <small>{API_BASE}</small>
          </div>
        </div>
        <div className="segmented">
          <button className={mode === "key" ? "active" : ""} onClick={() => setMode("key")}>API Key</button>
          <button className={mode === "login" ? "active" : ""} onClick={() => setMode("login")}>邮箱登录</button>
          <button className={mode === "register" ? "active" : ""} onClick={() => setMode("register")}>注册</button>
        </div>

        {mode === "key" && (
          <form onSubmit={(e) => { e.preventDefault(); void tryKey(key.trim()); }}>
            <label className="field">
              <span>API Key</span>
              <input value={key} onChange={(e) => setKey(e.target.value)} placeholder="sm_..." autoFocus />
            </label>
            <button className="primary" disabled={busy || !key.trim()}>进入</button>
          </form>
        )}

        {mode === "login" && (
          <form onSubmit={(e) => { e.preventDefault(); void login(); }}>
            <label className="field">
              <span>邮箱</span>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
            </label>
            <label className="field">
              <span>密码</span>
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
            </label>
            <button className="primary" disabled={busy}>登录</button>
          </form>
        )}

        {mode === "register" && !issued && (
          <form onSubmit={(e) => { e.preventDefault(); void register(); }}>
            <label className="field">
              <span>邮箱</span>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </label>
            <label className="field">
              <span>密码（至少 8 位，查看凭证时需要再次输入）</span>
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
            </label>
            <button className="primary" disabled={busy}>注册</button>
          </form>
        )}

        {issued && (
          <div className="issued">
            <p>账号已创建。下面的 API Key <strong>只显示这一次</strong>，请复制保存，并配置到各个智能体中。也可用邮箱密码登录控制台。</p>
            <code className="secret">{issued}</code>
            <div className="row">
              <button onClick={() => void navigator.clipboard.writeText(issued)}>复制</button>
              <button className="primary" onClick={() => onKey(issued)}>已保存，进入控制台</button>
            </div>
          </div>
        )}
        {error && <div className="error-line">{error}</div>}
        <p className="muted small">设备码登录用于本地守护进程与 CLI（smctl login）。</p>
      </div>
    </div>
  );
}
