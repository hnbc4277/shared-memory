import gsap from "gsap";

const EASE = { reveal: "expo.out", glide: "power4.out", settle: "back.out(1.6)", swap: "power2.inOut" };
const DUR = { slide: 0.52, press: 0.14, release: 0.46, hoverIn: 0.34 };

const RM = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const FINE = () => window.matchMedia("(hover: hover) and (pointer: fine)").matches;

const PRESS_SEL = ".btn, .nav-item, .module-tile, .cmd-chip, .agent-card.lift, .pearl-section.lift, .segmented button";
const LIFT_SEL = ".lift, .module-tile, .bento.m-glow";

/** 分段控件滑块（与样稿 bindSegmented / slideThumb axis=x 一致） */
export function syncSegmentThumb(seg: HTMLElement, activeBtn: HTMLElement) {
  const thumb = seg.querySelector(".seg-thumb") as HTMLElement | null;
  if (!thumb) return;

  const pr = seg.getBoundingClientRect();
  const tr = activeBtn.getBoundingClientRect();
  const x = tr.left - pr.left + seg.scrollLeft;
  const y = tr.top - pr.top + seg.scrollTop;

  if (RM()) {
    thumb.style.transform = `translate(${x}px, ${y}px)`;
    thumb.style.width = `${tr.width}px`;
    thumb.style.height = `${tr.height}px`;
    return;
  }

  gsap.to(thumb, {
    x,
    y,
    width: tr.width,
    height: tr.height,
    duration: DUR.slide,
    ease: EASE.glide,
    overwrite: true,
  });
  if (activeBtn.classList.contains("is-active")) {
    gsap.fromTo(activeBtn, { scale: 0.98 }, { scale: 1, duration: 0.35, ease: EASE.settle });
  }
}

function installPress(root: HTMLElement) {
  if (RM()) return () => {};
  const onDown = (e: PointerEvent) => {
    const el = (e.target as Element | null)?.closest(PRESS_SEL) as HTMLElement | null;
    if (!el || e.button !== 0 || el.classList.contains("is-loading")) return;
    gsap.to(el, { scale: 0.955, duration: DUR.press, ease: "power2.out", overwrite: "auto" });
  };
  const onUp = () => {
    gsap.to(PRESS_SEL, { scale: 1, duration: DUR.release, ease: EASE.settle, overwrite: "auto" });
  };
  root.addEventListener("pointerdown", onDown);
  window.addEventListener("pointerup", onUp);
  window.addEventListener("pointercancel", onUp);
  return () => {
    root.removeEventListener("pointerdown", onDown);
    window.removeEventListener("pointerup", onUp);
    window.removeEventListener("pointercancel", onUp);
  };
}

function installHoverLift(root: HTMLElement) {
  if (RM() || !FINE()) return () => {};
  const onOver = (e: PointerEvent) => {
    const el = (e.target as Element | null)?.closest(LIFT_SEL) as HTMLElement | null;
    if (!el || el.contains(e.relatedTarget as Node)) return;
    gsap.to(el, { y: -3, duration: DUR.hoverIn, ease: EASE.glide, overwrite: "auto" });
  };
  const onOut = (e: PointerEvent) => {
    const el = (e.target as Element | null)?.closest(LIFT_SEL) as HTMLElement | null;
    if (!el || el.contains(e.relatedTarget as Node)) return;
    gsap.to(el, { y: 0, duration: 0.42, ease: EASE.settle, overwrite: "auto" });
  };
  root.addEventListener("pointerover", onOver);
  root.addEventListener("pointerout", onOut);
  return () => {
    root.removeEventListener("pointerover", onOver);
    root.removeEventListener("pointerout", onOut);
  };
}

function bindTopbarScroll(scroller: HTMLElement, workspace: HTMLElement) {
  const onScroll = () => {
    workspace.classList.toggle("is-scrolled", scroller.scrollTop > 8);
  };
  onScroll();
  scroller.addEventListener("scroll", onScroll, { passive: true });
  return () => scroller.removeEventListener("scroll", onScroll);
}

export type PearlMicroOptions = {
  root: HTMLElement | null;
  scrollEl?: Element | null;
  workspaceEl?: Element | null;
};

/** 样稿 pearl-runtime：按压 · 悬停抬升 · 顶栏滚动凝结 */
export function installPearlMicroInteractions({ root, scrollEl, workspaceEl }: PearlMicroOptions): () => void {
  if (!root) return () => {};
  const cleanups = [installPress(root), installHoverLift(root)];
  if (scrollEl instanceof HTMLElement && workspaceEl instanceof HTMLElement) {
    cleanups.push(bindTopbarScroll(scrollEl, workspaceEl));
  }
  return () => cleanups.forEach((fn) => fn());
}
