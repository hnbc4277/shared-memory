import { useEffect, useId, type ReactNode } from "react";

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

export function Section({ title, actions, children, id }: { title: string; actions?: ReactNode; children: ReactNode; id?: string }) {
  return (
    <section className="card" id={id}>
      <header className="card-head">
        <h2>{title}</h2>
        {actions ? <div className="row">{actions}</div> : null}
      </header>
      {children}
    </section>
  );
}

export function StatRow({ children }: { children: ReactNode }) {
  return <div className="stat-row">{children}</div>;
}

export function StatTile({
  label,
  value,
  hint,
  tone = "neutral",
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: "neutral" | "ok" | "warn" | "primary";
}) {
  return (
    <div className={`stat-tile tone-${tone}`}>
      <div className="stat-tile-label">{label}</div>
      <div className="stat-tile-value">{value}</div>
      {hint ? <div className="stat-tile-hint">{hint}</div> : null}
    </div>
  );
}

export function PageStack({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={className ? `stack ${className}` : "stack"}>{children}</div>;
}

export function DataTable({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={className ? `tbl-wrap ${className}` : "tbl-wrap"}>
      <table className="tbl">{children}</table>
    </div>
  );
}

export function Modal({
  title,
  children,
  onClose,
  wide,
  footer,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  footer?: ReactNode;
}) {
  const titleId = useId();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="modal-backdrop" onClick={onClose} role="presentation">
      <div
        className={wide ? "modal modal-wide" : "modal"}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-head">
          <h3 id={titleId}>{title}</h3>
          <button type="button" className="ghost" onClick={onClose} aria-label="关闭">
            关闭
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer ? <div className="modal-foot">{footer}</div> : null}
      </div>
    </div>
  );
}

export function ConfirmModal({
  title = "确认",
  message,
  confirmLabel = "确认",
  danger,
  onConfirm,
  onCancel,
  children,
}: {
  title?: string;
  message: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  children?: ReactNode;
}) {
  return (
    <Modal
      title={title}
      onClose={onCancel}
      footer={
        <div className="row">
          <button type="button" className={danger ? "btn danger" : "btn primary"} onClick={onConfirm}>
            {confirmLabel}
          </button>
          <button type="button" className="ghost" onClick={onCancel}>取消</button>
        </div>
      }
    >
      <div>{message}</div>
      {children}
    </Modal>
  );
}

export function CodeBlock({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="code-block">
      <span className="code-block-label">{label}</span>
      {children}
    </div>
  );
}
