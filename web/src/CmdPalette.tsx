import gsap from "gsap";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

export type CmdNavItem = { id: string; label: string; icon: string; group?: string };

const RM = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

export default function CmdPalette({
  open,
  onOpenChange,
  items,
  onPick,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  items: CmdNavItem[];
  onPick: (id: string) => void;
}) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [q, setQ] = useState("");
  const [activeIdx, setActiveIdx] = useState(0);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return items;
    return items.filter(
      (it) =>
        it.label.toLowerCase().includes(needle) ||
        it.id.toLowerCase().includes(needle) ||
        (it.group?.toLowerCase().includes(needle) ?? false),
    );
  }, [items, q]);

  const close = useCallback(() => {
    const overlay = overlayRef.current;
    const panel = panelRef.current;
    if (!overlay) {
      onOpenChange(false);
      return;
    }
    if (RM() || !panel) {
      overlay.classList.remove("is-open");
      onOpenChange(false);
      return;
    }
    gsap.to(overlay, {
      opacity: 0,
      duration: 0.2,
      ease: "power2.in",
      onComplete: () => {
        overlay.classList.remove("is-open");
        onOpenChange(false);
      },
    });
  }, [onOpenChange]);

  const pick = useCallback(
    (id: string) => {
      onPick(id);
      setQ("");
      close();
    },
    [close, onPick],
  );

  useEffect(() => {
    if (!open) return;
    setQ("");
    setActiveIdx(0);
    const overlay = overlayRef.current;
    const panel = panelRef.current;
    if (!overlay || !panel) return;
    overlay.classList.add("is-open");
    if (RM()) {
      gsap.set(overlay, { opacity: 1 });
      inputRef.current?.focus();
      return;
    }
    gsap.set(overlay, { opacity: 0 });
    gsap.set(panel, { transformOrigin: "50% 20%", scale: 0.86, opacity: 0, y: 10 });
    gsap.to(overlay, { opacity: 1, duration: 0.22, ease: "power1.out" });
    gsap.to(panel, {
      scale: 1,
      opacity: 1,
      y: 0,
      duration: 0.42,
      ease: "expo.out",
      clearProps: "transform",
      onComplete: () => inputRef.current?.focus(),
    });
  }, [open]);

  useEffect(() => {
    setActiveIdx((i) => Math.min(i, Math.max(0, filtered.length - 1)));
  }, [filtered.length]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setActiveIdx((i) => (filtered.length ? (i + 1) % filtered.length : 0));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setActiveIdx((i) => (filtered.length ? (i - 1 + filtered.length) % filtered.length : 0));
        return;
      }
      if (e.key === "Enter" && filtered[activeIdx]) {
        e.preventDefault();
        pick(filtered[activeIdx].id);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, close, filtered, activeIdx, pick]);

  return (
    <div
      className={`cmd-overlay${open ? " is-open" : ""}`}
      ref={overlayRef}
      aria-hidden={!open}
      onClick={(e) => {
        if (e.target === overlayRef.current) close();
      }}
    >
      <div className="cmd-panel m-float" ref={panelRef} role="dialog" aria-modal="true" aria-label="命令面板">
        <input
          ref={inputRef}
          type="text"
          placeholder="输入页面名称…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <div role="listbox">
          {filtered.map((it, idx) => (
            <div
              key={it.id}
              role="option"
              className={`cmd-item${idx === activeIdx ? " is-active" : ""}`}
              aria-selected={idx === activeIdx}
              onMouseEnter={() => setActiveIdx(idx)}
              onClick={() => pick(it.id)}
            >
              <span className="nav-ico" aria-hidden>
                {it.icon}
              </span>
              {it.group ? (
                <span>
                  {it.label}
                  <span className="cmd-item-sub"> · {it.group}</span>
                </span>
              ) : (
                it.label
              )}
            </div>
          ))}
          {filtered.length === 0 && <div className="cmd-empty">没有匹配的页面</div>}
        </div>
      </div>
    </div>
  );
}
