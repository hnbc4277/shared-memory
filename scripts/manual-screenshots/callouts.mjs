/** @typedef {{ id: string, label: string, role?: string, name?: string, selector?: string, exact?: boolean }} Callout */

/** @param {import('playwright').Page} page @param {Callout[]} callouts */
export async function applyCallouts(page, callouts) {
  const resolved = [];
  for (const c of callouts) {
    let loc;
    if (c.selector) {
      loc = page.locator(c.selector).first();
    } else if (c.name) {
      loc = page.getByRole(c.role ?? "button", { name: c.name, exact: c.exact ?? false }).first();
    } else {
      continue;
    }
    try {
      if ((await loc.count()) === 0) {
        console.warn(`[callout] 未找到 ${c.id} ${c.label}`);
        continue;
      }
      await loc.scrollIntoViewIfNeeded({ timeout: 3000 }).catch(() => {});
      const box = await loc.boundingBox();
      if (!box || box.width < 1 || box.height < 1) {
        console.warn(`[callout] 无框 ${c.id}`);
        continue;
      }
      resolved.push({ id: c.id, label: c.label, box });
    } catch (e) {
      console.warn(`[callout] ${c.id}: ${e.message}`);
    }
  }

  await page.evaluate((items) => {
    document.getElementById("manual-shot-layer")?.remove();
    const style = document.createElement("style");
    style.id = "manual-shot-style";
    style.textContent = `
      #manual-shot-layer { position: fixed; inset: 0; pointer-events: none; z-index: 99999; }
      .manual-pin {
        position: fixed; box-sizing: border-box;
        border: 2px solid #0066cc; border-radius: 6px;
        box-shadow: 0 0 0 2px rgba(0,102,204,0.15);
      }
      .manual-badge {
        position: fixed; min-width: 22px; height: 22px; padding: 0 6px;
        background: #0066cc; color: #fff; font: 600 11px/22px "PingFang SC","Microsoft YaHei",sans-serif;
        border-radius: 11px; text-align: center; transform: translate(-50%, -50%);
      }
      .manual-legend {
        position: fixed; left: 12px; right: 12px; bottom: 12px; max-height: 38vh; overflow: auto;
        background: rgba(255,255,255,0.96); border: 1px solid #e5e5e7; border-radius: 12px;
        padding: 10px 14px; font: 12px/1.45 "PingFang SC","Microsoft YaHei",sans-serif;
        color: #1d1d1f; box-shadow: 0 8px 28px rgba(0,0,0,0.12);
      }
      .manual-legend h4 { margin: 0 0 8px; font-size: 13px; font-weight: 600; }
      .manual-legend dl { margin: 0; display: grid; grid-template-columns: auto 1fr; gap: 4px 10px; }
      .manual-legend dt { font-weight: 600; color: #0066cc; }
      .manual-legend dd { margin: 0; }
    `;
    if (!document.getElementById("manual-shot-style")) document.head.appendChild(style);

    const layer = document.createElement("div");
    layer.id = "manual-shot-layer";

    for (const it of items) {
      const { box, id, label } = it;
      const pin = document.createElement("div");
      pin.className = "manual-pin";
      pin.style.left = `${box.x - 2}px`;
      pin.style.top = `${box.y - 2}px`;
      pin.style.width = `${box.width + 4}px`;
      pin.style.height = `${box.height + 4}px`;
      layer.appendChild(pin);

      const badge = document.createElement("div");
      badge.className = "manual-badge";
      badge.textContent = id.replace(/^[A-Z]+-/, "").replace(/^L-/, "");
      badge.style.left = `${box.x}px`;
      badge.style.top = `${box.y}px`;
      layer.appendChild(badge);
    }

    const legend = document.createElement("div");
    legend.className = "manual-legend";
    legend.innerHTML = `<h4>图中编号说明</h4><dl>${items
      .map((it) => `<dt>${it.id}</dt><dd>${it.label}</dd>`)
      .join("")}</dl>`;
    layer.appendChild(legend);
    document.body.appendChild(layer);
  }, resolved);

  return resolved.length;
}

export async function clearCallouts(page) {
  await page.evaluate(() => {
    document.getElementById("manual-shot-layer")?.remove();
  });
}
