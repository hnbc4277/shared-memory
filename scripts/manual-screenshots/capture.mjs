import { chromium } from "playwright";
import { copyFileSync, existsSync, mkdirSync, readdirSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { applyCallouts, clearCallouts } from "./callouts.mjs";
import { SCENES } from "./scenes.mjs";
import { seedAccount } from "./seed.mjs";

const __dir = dirname(fileURLToPath(import.meta.url));
const OUT = join(__dir, "../../docs/公开/manual/images");
const WEB = (process.env.MANUAL_WEB_URL ?? "http://127.0.0.1:15173").replace(/\/+$/, "");

const KEY_STORAGE = "sm.apiKey";
const AUTH_MODE_STORAGE = "sm.authMode";

async function gotoTab(page, hash) {
  // SSE / agents stream keeps networkidle from ever settling
  await page.goto(`${WEB}/#${hash}`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(400);
}

/** @param {import('playwright').BrowserContext} ctx @param {string} apiKey */
async function openAuthedPage(ctx, hash) {
  const page = await ctx.newPage();
  await page.goto(`${WEB}/#${hash ?? "overview"}`, { waitUntil: "domcontentloaded" });
  await page.waitForSelector(".app-frame", { timeout: 25000 });
  return page;
}

function syncToPublic() {
  const pub = join(__dir, "../../web/public/docs/manual/images");
  mkdirSync(pub, { recursive: true });
  for (const f of readdirSync(OUT)) {
    if (f.endsWith(".png")) copyFileSync(join(OUT, f), join(pub, f));
  }
  console.log(`已同步 PNG → web/public/docs/manual/images/`);
}

async function main() {
  if (!existsSync(OUT)) mkdirSync(OUT, { recursive: true });

  console.log("准备演示数据…");
  const seed = await seedAccount();

  const browser = await chromium.launch({ headless: true });
  const guestCtx = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
    deviceScaleFactor: 2,
  });
  const authCtx = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
    deviceScaleFactor: 2,
  });
  await authCtx.addInitScript(
    ([key, keyName, modeName]) => {
      localStorage.setItem(keyName, key);
      localStorage.setItem(modeName, "key");
    },
    [seed.apiKey, KEY_STORAGE, AUTH_MODE_STORAGE],
  );

  for (const scene of SCENES) {
    console.log(`截图 ${scene.file} …`);
    let page;
    try {
      if (scene.auth) {
        page = await openAuthedPage(authCtx, scene.hash ?? "history");
      } else {
        page = await guestCtx.newPage();
        await page.goto(WEB, { waitUntil: "domcontentloaded" });
      }
      if (scene.setup) await scene.setup(page, seed);
      await page.waitForTimeout(600);
      await clearCallouts(page);
      const n = await applyCallouts(page, scene.callouts);
      await page.waitForTimeout(200);
      await page.screenshot({ path: join(OUT, scene.file), fullPage: scene.fullPage ?? true });
      console.log(`  ✓ ${scene.file}（标注 ${n}/${scene.callouts.length}）`);
    } catch (e) {
      console.error(`  ✗ ${scene.file}: ${e.message}`);
    } finally {
      if (page) await page.close().catch(() => {});
    }
  }

  await guestCtx.close();
  await authCtx.close();
  await browser.close();
  syncToPublic();
  console.log("完成。");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
