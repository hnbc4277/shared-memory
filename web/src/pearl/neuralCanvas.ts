/** Canvas neural background (from design-explore/pearl-runtime.js). */
export type NeuralDensity = "light" | "medium" | "heavy";

export function startNeural(canvas: HTMLCanvasElement | null, density: NeuralDensity = "heavy") {
  if (!canvas) return { burst: () => {}, destroy: () => {} };

  const ctxRaw = canvas.getContext("2d");
  if (!ctxRaw) return { burst: () => {}, destroy: () => {} };
  const ctx: CanvasRenderingContext2D = ctxRaw;

  const nodes: { x: number; y: number; ox: number; oy: number; r: number; phase: number }[] = [];
  const edges: { a: number; b: number; pulse: number }[] = [];
  const N = density === "light" ? 28 : density === "heavy" ? 64 : 48;
  let w = 0;
  let h = 0;
  const t0 = performance.now();
  let burstAt = -1;
  let raf = 0;
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  const el = canvas;

  function resize() {
    const parent = el.parentElement;
    if (!parent) return;
    const rect = parent.getBoundingClientRect();
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    w = rect.width;
    h = rect.height;
    el.width = w * dpr;
    el.height = h * dpr;
    el.style.width = `${w}px`;
    el.style.height = `${h}px`;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    nodes.length = 0;
    for (let i = 0; i < N; i++) {
      nodes.push({
        x: 0.08 * w + Math.random() * 0.84 * w,
        y: 0.1 * h + Math.random() * 0.8 * h,
        ox: 0,
        oy: 0,
        r: 2 + Math.random() * 2.5,
        phase: Math.random() * Math.PI * 2,
      });
    }
    edges.length = 0;
    const maxDist = density === "heavy" ? 140 : 120;
    for (let i = 0; i < N; i++) {
      for (let j = i + 1; j < N; j++) {
        const dist = Math.hypot(nodes[i].x - nodes[j].x, nodes[i].y - nodes[j].y);
        const prob = density === "light" ? 0.22 : 0.35;
        if (dist < maxDist && Math.random() < prob) edges.push({ a: i, b: j, pulse: Math.random() });
      }
    }
    nodes.forEach((n) => {
      n.ox = n.x;
      n.oy = n.y;
    });
  }

  function frame(now: number) {
    const t = (now - t0) / 1000;
    const cx = w * 0.55;
    const cy = h * 0.45;
    let burstK = -1;
    if (burstAt >= 0) {
      burstK = (now - burstAt) / 1100;
      if (burstK >= 1) burstAt = -1;
    }

    ctx.clearRect(0, 0, w, h);

    if (burstK >= 0 && burstK < 1) {
      const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, 120 + burstK * 80);
      g.addColorStop(0, `rgba(120, 224, 36, ${0.42 * (1 - burstK)})`);
      g.addColorStop(1, "transparent");
      ctx.fillStyle = g;
      ctx.fillRect(0, 0, w, h);
    }

    const lineAlpha = density === "heavy" ? 0.22 : 0.17;
    for (const e of edges) {
      const na = nodes[e.a];
      const nb = nodes[e.b];
      ctx.strokeStyle = `rgba(120, 224, 36, ${lineAlpha})`;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(na.x, na.y);
      ctx.lineTo(nb.x, nb.y);
      ctx.stroke();
      if (!reduced && (burstK < 0 || burstK > 0.85)) {
        const p = (t * 0.4 + e.pulse) % 1;
        ctx.fillStyle = "rgba(140, 235, 55, 0.9)";
        ctx.beginPath();
        ctx.arc(na.x + (nb.x - na.x) * p, na.y + (nb.y - na.y) * p, 2, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    for (const n of nodes) {
      if (burstK >= 0 && burstK < 0.7) {
        const pull = Math.sin(burstK * Math.PI) * 0.72;
        n.x = n.ox + (cx - n.ox) * pull;
        n.y = n.oy + (cy - n.oy) * pull;
      } else {
        n.x = n.ox;
        n.y = n.oy;
      }
      const breathe = reduced ? 1 : 0.85 + 0.15 * Math.sin(t * 0.8 + n.phase);
      ctx.fillStyle = `rgba(242, 245, 244, ${0.35 * breathe})`;
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r * breathe, 0, Math.PI * 2);
      ctx.fill();
    }

    if (burstK >= 0.55 && burstK < 0.95) {
      const ck = (burstK - 0.55) / 0.4;
      ctx.strokeStyle = `rgba(140, 235, 55, ${0.95 * ck})`;
      ctx.lineWidth = 3;
      ctx.lineCap = "round";
      ctx.lineJoin = "round";
      ctx.beginPath();
      ctx.moveTo(cx - 22, cy + 2);
      ctx.lineTo(cx - 6, cy + 18);
      ctx.lineTo(cx + 26, cy - 16);
      ctx.stroke();
    }

    raf = requestAnimationFrame(frame);
  }

  window.addEventListener("resize", resize);
  resize();
  raf = requestAnimationFrame(frame);

  return {
    burst() {
      burstAt = performance.now();
    },
    destroy() {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", resize);
    },
  };
}
