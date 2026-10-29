import gsap from "gsap";

const EASE = { reveal: "expo.out", glide: "power4.out", settle: "back.out(1.6)", swap: "power2.inOut" };
const DUR = { slide: 0.52, pageIn: 0.62 };

const RM = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const FINE = () => window.matchMedia("(hover: hover) and (pointer: fine)").matches;

export function buildDialTicks(svg: SVGSVGElement | null) {
  if (!svg) return;
  svg.innerHTML = "";
  for (let i = 0; i < 60; i++) {
    const a = (i / 60) * Math.PI * 2 - Math.PI / 2;
    const major = i % 5 === 0;
    const r1 = major ? 40 : 43;
    const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line.setAttribute("class", `dial-tick${major ? " is-major" : ""}`);
    line.setAttribute("data-i", String(i));
    line.setAttribute("x1", String(50 + Math.cos(a) * r1));
    line.setAttribute("y1", String(50 + Math.sin(a) * r1));
    line.setAttribute("x2", String(50 + Math.cos(a) * 47));
    line.setAttribute("y2", String(50 + Math.sin(a) * 47));
    svg.appendChild(line);
  }
}

export function tickDialClock(svg: SVGSVGElement | null, timeEl: HTMLElement | null, dateEl: HTMLElement | null) {
  const d = new Date();
  if (timeEl) timeEl.textContent = d.toTimeString().slice(0, 8);
  if (dateEl) {
    const days = "SUN MON TUE WED THU FRI SAT".split(" ");
    dateEl.textContent = `${d.getFullYear()}.${String(d.getMonth() + 1).padStart(2, "0")}.${String(d.getDate()).padStart(2, "0")} · ${days[d.getDay()]}`;
  }
  if (!svg) return;
  const sec = d.getSeconds();
  svg.querySelectorAll(".dial-tick").forEach((el) => {
    el.classList.toggle("is-on", Number(el.getAttribute("data-i")) === sec);
  });
}

export function playLoginIntro(root: HTMLElement | null) {
  if (!root || RM()) {
    root?.classList.add("is-entrance-ready");
    return;
  }
  const q = (s: string) => [...root.querySelectorAll(s)];
  const tl = gsap.timeline({ paused: true });
  tl.set([...q(".login-brand"), ...q(".hero-line"), ...q(".login-card"), ...q(".login-plate"), ...q(".login-dial")], {
    opacity: 0,
    y: 16,
  });
  tl.set(q(".dial-tick"), { opacity: 0.15 });
  tl.add(() => root.classList.add("is-entrance-ready"));
  tl.to(q(".login-brand"), { opacity: 1, y: 0, duration: 0.7, ease: EASE.reveal }, 0);
  tl.to(q(".hero-line"), { opacity: 1, y: 0, duration: 0.9, ease: EASE.reveal, stagger: 0.06 }, 0.12);
  tl.to(q(".login-dial"), { opacity: 1, y: 0, duration: 0.9, ease: EASE.reveal }, 0.2);
  tl.to(q(".dial-tick"), { opacity: 1, duration: 0.3, stagger: { each: 0.008, from: "start" } }, 0.3);
  tl.to(q(".cap-item"), { opacity: 1, y: 0, duration: 0.7, ease: EASE.reveal, stagger: 0.06 }, 0.36);
  tl.to(q(".login-plate.is-health"), { opacity: 1, y: 0, duration: 0.7, ease: EASE.reveal }, 0.5);
  tl.to(q(".login-card"), { opacity: 1, y: 0, duration: 0.8, ease: EASE.glide }, 0.22);
  const ff = () => tl.timeScale(6);
  window.addEventListener("pointerdown", ff, { once: true });
  window.addEventListener("keydown", ff, { once: true });
  requestAnimationFrame(() => requestAnimationFrame(() => tl.play(0)));
}

export function playSuccessOrb(slot: HTMLElement | null, btn: HTMLElement | null): Promise<void> {
  return new Promise((resolve) => {
    if (RM() || !slot || !btn) {
      resolve();
      return;
    }
    const orb = slot.querySelector(".success-orb") as HTMLElement | null;
    if (!orb) {
      resolve();
      return;
    }
    const ring = orb.querySelector("circle");
    const check = orb.querySelector("path");
    const startW = btn.offsetWidth || slot.clientWidth;

    btn.style.display = "none";
    orb.classList.add("is-visible");
    gsap.set(orb, { width: startW, scale: 1 });
    if (ring) gsap.set(ring, { strokeDashoffset: 126 });
    if (check) gsap.set(check, { strokeDashoffset: 24 });

    const tl = gsap.timeline({ onComplete: resolve });
    tl.to(orb, { width: 46, duration: 0.42, ease: EASE.swap });
    if (ring) tl.to(ring, { strokeDashoffset: 0, duration: 0.5, ease: EASE.reveal }, "-=0.08");
    if (check) tl.to(check, { strokeDashoffset: 0, duration: 0.32, ease: EASE.settle }, "-=0.22");
    tl.to(orb, { scale: 1.06, duration: 0.16, ease: "power1.out" }).to(orb, { scale: 1, duration: 0.3, ease: EASE.settle });
  });
}

export function slideNavThumb(thumb: HTMLElement | null, list: HTMLElement | null, target: HTMLElement | null) {
  if (RM() || !thumb || !list || !target) return;
  const pr = list.getBoundingClientRect();
  const tr = target.getBoundingClientRect();
  gsap.to(thumb, {
    y: tr.top - pr.top + list.scrollTop,
    height: tr.height,
    duration: DUR.slide,
    ease: EASE.glide,
    overwrite: true,
  });
}

export function playShellEntrance(frame: HTMLElement | null) {
  if (!frame || RM()) {
    frame?.classList.add("is-entrance-ready");
    return;
  }
  frame.classList.remove("is-entrance-ready");
  const brand = frame.querySelectorAll(".rail-brand");
  const groups = frame.querySelectorAll(".nav-group");
  const top = frame.querySelectorAll(".topbar > *");
  const tl = gsap.timeline();
  tl.set([...brand, ...groups, ...top], { opacity: 0, y: 8 });
  tl.add(() => frame.classList.add("is-entrance-ready"));
  tl.to(brand, { opacity: 1, y: 0, duration: 0.6, ease: EASE.reveal }, 0);
  tl.to(groups, { opacity: 1, y: 0, duration: 0.6, ease: EASE.reveal, stagger: { each: 0.06, from: "start" } }, 0.08);
  tl.to(top, { opacity: 1, y: 0, duration: 0.5, ease: EASE.reveal, stagger: 0.04 }, 0.14);
}

export function bindLoginCardTilt(card: HTMLElement | null) {
  if (!card || RM() || !FINE()) return () => {};
  const tiltX = gsap.quickTo(card, "rotationX", { duration: 0.6, ease: "power3.out" });
  const tiltY = gsap.quickTo(card, "rotationY", { duration: 0.6, ease: "power3.out" });
  card.style.perspective = "900px";
  const onMove = (e: PointerEvent) => {
    const r = card.getBoundingClientRect();
    const px = (e.clientX - r.left) / r.width - 0.5;
    const py = (e.clientY - r.top) / r.height - 0.5;
    tiltY(Math.max(-1, Math.min(1, px * 2)) * 4);
    tiltX(Math.max(-1, Math.min(1, py * 2)) * -3);
    card.style.setProperty("--px", `${e.clientX - r.left}px`);
    card.style.setProperty("--py", `${e.clientY - r.top}px`);
  };
  const onLeave = () => {
    tiltX(0);
    tiltY(0);
  };
  card.addEventListener("pointermove", onMove);
  card.addEventListener("pointerleave", onLeave);
  return () => {
    card.removeEventListener("pointermove", onMove);
    card.removeEventListener("pointerleave", onLeave);
  };
}

export function pageEnter(page: HTMLElement | null) {
  if (!page) return;
  if (RM()) {
    page.classList.add("is-entrance-ready");
    page.closest(".page")?.classList.add("is-entrance-ready");
    return;
  }
  const items = page.querySelectorAll(".reveal");
  const cards = page.querySelectorAll(".bento, .agent-card.lift, .pearl-section.lift");
  gsap.set(items, { opacity: 0, y: 12 });
  gsap.set(cards, { opacity: 0, y: 18 });
  const tl = gsap.timeline({
    onComplete: () => {
      page.classList.add("is-entrance-ready");
      page.closest(".page")?.classList.add("is-entrance-ready");
    },
  });
  tl.add(() => page.classList.add("is-entrance-ready"), 0);
  tl.to(items, {
    opacity: 1,
    y: 0,
    duration: DUR.pageIn,
    ease: EASE.reveal,
    stagger: 0.05,
    clearProps: "transform",
  });
  if (cards.length) {
    tl.to(
      cards,
      {
        opacity: 1,
        y: 0,
        duration: 0.72,
        ease: EASE.reveal,
        stagger: { each: 0.045, from: "center" },
        clearProps: "transform",
      },
      0.08,
    );
  }
}

export const SHELL_ENTRANCE_KEY = "sm.pearlShellEntrance";
