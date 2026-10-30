#!/usr/bin/env node
/**
 * Ensures web/public adapter README copies match repo sources.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const maps = [
  { src: "adapters/hermes/README.md", dst: "web/public/adapters/hermes/README.md" },
  { src: "adapters/openclaw/README.md", dst: "web/public/adapters/openclaw/README.md" },
  { src: "adapters/deepseek-harness/README.md", dst: "web/public/adapters/deepseek-harness/README.md" },
  { src: "adapters/doubao/README.md", dst: "web/public/adapters/doubao/README.md" },
];

let failed = false;
for (const m of maps) {
  const srcPath = path.join(root, m.src);
  const dstPath = path.join(root, m.dst);
  if (!fs.existsSync(srcPath)) {
    console.error(`missing source: ${m.src}`);
    failed = true;
    continue;
  }
  if (!fs.existsSync(dstPath)) {
    console.error(`missing public copy: ${m.dst} (run scripts/sync-web-docs.ps1)`);
    failed = true;
    continue;
  }
  const a = fs.readFileSync(srcPath);
  const b = fs.readFileSync(dstPath);
  if (!a.equals(b)) {
    console.error(`out of sync: ${m.dst} ≠ ${m.src}`);
    failed = true;
  }
}

if (failed) {
  process.exit(1);
}
console.log("web/public adapter docs match sources");
