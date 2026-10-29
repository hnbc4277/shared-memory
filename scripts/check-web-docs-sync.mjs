#!/usr/bin/env node
/**
 * Ensures web/public doc copies match repo sources (same paths as sync-web-docs.ps1).
 * Used in CI on Linux where PowerShell sync is optional.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function findDocsBucket(markerRel) {
  const docsRoot = path.join(root, "docs");
  if (!fs.existsSync(docsRoot)) return null;
  for (const name of fs.readdirSync(docsRoot)) {
    const dir = path.join(docsRoot, name);
    if (!fs.statSync(dir).isDirectory()) continue;
    const candidate = path.join(dir, markerRel);
    if (fs.existsSync(candidate)) return dir;
  }
  return null;
}

const privateDocs = findDocsBucket(path.join("superpowers", "specs"));
if (!privateDocs) {
  console.error("Cannot find docs bucket containing superpowers/specs");
  process.exit(1);
}

const maps = [
  { src: "adapters/hermes/README.md", dst: "web/public/adapters/hermes/README.md" },
  { src: "adapters/openclaw/README.md", dst: "web/public/adapters/openclaw/README.md" },
  { src: "adapters/deepseek-harness/README.md", dst: "web/public/adapters/deepseek-harness/README.md" },
  {
    src: path.join(privateDocs, "superpowers", "specs", "2026-09-30-shared-memory-design.md"),
    dst: "web/public/docs/superpowers/specs/2026-09-30-shared-memory-design.md",
    absoluteSrc: true,
  },
  {
    src: path.join(privateDocs, "superpowers", "specs", "2026-10-12-agent-connections-design.md"),
    dst: "web/public/docs/superpowers/specs/2026-10-12-agent-connections-design.md",
    absoluteSrc: true,
  },
];

let failed = false;
for (const m of maps) {
  const srcPath = m.absoluteSrc ? m.src : path.join(root, m.src);
  const dstPath = path.join(root, m.dst);
  const srcLabel = m.absoluteSrc ? path.relative(root, m.src) : m.src;
  if (!fs.existsSync(srcPath)) {
    console.error(`missing source: ${srcLabel}`);
    failed = true;
    continue;
  }
  if (!fs.existsSync(dstPath)) {
    console.error(`missing public copy: ${m.dst} (run scripts/sync-web-docs.ps1 or web npm run sync-docs)`);
    failed = true;
    continue;
  }
  const a = fs.readFileSync(srcPath);
  const b = fs.readFileSync(dstPath);
  if (!a.equals(b)) {
    console.error(`out of sync: ${m.dst} ≠ ${srcLabel}`);
    failed = true;
  }
}

if (failed) {
  process.exit(1);
}
console.log("web/public doc copies match sources");
