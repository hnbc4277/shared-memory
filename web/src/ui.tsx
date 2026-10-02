import type { ReactNode } from "react";

export function Chip({ children, tone = "neutral", title }: { children: ReactNode; tone?: "neutral" | "agent" | "skill" | "plugin" | "tool" | "warn"; title?: string }) {
  return (
    <span className={`chip chip-${tone}`} title={title}>
      {children}
    </span>
  );
}

export function ChipList({ label, items, tone }: { label: string; items: string[]; tone: "skill" | "plugin" | "tool" }) {
  return (
    <span className="chiplist">
      <span className="muted">{label}</span>
      {items.length === 0 ? <span className="muted">无</span> : items.map((s) => <Chip key={s} tone={tone}>{s}</Chip>)}
    </span>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}

export function ErrorLine({ error }: { error: string | null }) {
  return error ? <div className="error-line">{error}</div> : null;
}

export function Toggle({ label, hint, checked, onChange }: { label: string; hint: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="toggle">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="toggle-track" aria-hidden />
      <span>
        <strong>{label}</strong>
        <small>{hint}</small>
      </span>
    </label>
  );
}

export function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="card">
      <header className="card-head">
        <h2>{title}</h2>
        <div className="row">{actions}</div>
      </header>
      {children}
    </section>
  );
}
