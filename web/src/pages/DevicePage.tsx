import { useEffect, useState } from "react";
import { useApp } from "../ctx";
import { ErrorLine, Section } from "../ui";

export default function DevicePage() {
  const { api, notify } = useApp();
  const [userCode, setUserCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const q = new URLSearchParams(location.search).get("user_code");
    if (q) setUserCode(q.replace(/-/g, "").toUpperCase());
  }, []);

  async function approve(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post("/v1/device/approve", { user_code: userCode });
      setDone(true);
      notify("设备已授权，可回到 CLI 继续");
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      <Section title="授权设备">
        <p className="muted small">
          在 CLI 运行 <code>smctl login</code> 后会显示一串用户码。确认是你本人操作后再授权；授权后该设备会获得一把可吊销的 API Key。
        </p>
        {done ? (
          <p>已授权。可以关闭此页，回到终端等待 <code>smctl</code> 完成。</p>
        ) : (
          <form className="row" onSubmit={(e) => void approve(e)}>
            <input
              className="wide"
              value={userCode}
              onChange={(e) => setUserCode(e.target.value.toUpperCase())}
              placeholder="用户码，例如 ABCD1234"
              autoFocus
              required
            />
            <button className="primary" disabled={busy || userCode.trim().length < 8}>
              {busy ? "授权中…" : "授权此设备"}
            </button>
          </form>
        )}
        <ErrorLine error={error} />
      </Section>
    </div>
  );
}
