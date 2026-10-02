import assert from "node:assert/strict";
import http from "node:http";
import { after, before, test } from "node:test";
import { APIError, Client } from "./index.ts";

let server: http.Server;
let baseUrl: string;
let lastAuth = "";
let lastIngest: unknown;

before(async () => {
  server = http.createServer((req, res) => {
    lastAuth = String(req.headers.authorization ?? "");
    const url = new URL(req.url ?? "/", "http://127.0.0.1");
    const chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      const body = Buffer.concat(chunks).toString("utf8");
      if (req.method === "POST" && url.pathname === "/v1/ingest") {
        lastIngest = JSON.parse(body);
        res.writeHead(201, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify({
            session_id: "s1",
            message_id: "m1",
            seq: 1,
            warnings: [],
            prompts: [],
          }),
        );
        return;
      }
      if (req.method === "POST" && url.pathname === "/v1/recall") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify({
            profile: "p",
            running_summary: "",
            hits: [],
            context: "ctx",
            inject_enabled: true,
            warnings: [],
          }),
        );
        return;
      }
      if (req.method === "GET" && url.pathname === "/v1/sessions") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify([{ id: "s1", agent: "cursor" }]));
        return;
      }
      if (req.method === "GET" && url.pathname === "/v1/candidates") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ vault: [], artifacts: [] }));
        return;
      }
      if (req.method === "POST" && url.pathname.startsWith("/v1/vault/candidates/")) {
        res.writeHead(204);
        res.end();
        return;
      }
      res.writeHead(404);
      res.end("not found");
    });
  });
  await new Promise<void>((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve());
  });
  const addr = server.address();
  if (!addr || typeof addr === "string") throw new Error("no addr");
  baseUrl = `http://127.0.0.1:${addr.port}`;
});

after(async () => {
  await new Promise<void>((resolve, reject) => {
    server.close((err) => (err ? reject(err) : resolve()));
  });
});

test("Client ingest/recall/candidates with Bearer", async () => {
  const c = new Client({ baseUrl, apiKey: "k" });
  const ing = await c.ingest({
    agent: "cursor",
    skills: [],
    plugins: [],
    tools: [],
    role: "user",
    content: "hi",
    external_session_id: "cursor:1",
  });
  assert.equal(lastAuth, "Bearer k");
  assert.equal((lastIngest as { content: string }).content, "hi");
  assert.equal(ing.session_id, "s1");

  const rec = await c.recall({ query: "q", top_k: 3 });
  assert.equal(rec.context, "ctx");

  const sessions = await c.listSessions({ agent: "cursor" });
  assert.equal(sessions[0]?.id, "s1");

  const cands = await c.listCandidates();
  assert.deepEqual(cands.vault, []);
  await c.resolveCandidate("vault", "c1", "reject");
});

test("APIError on 401", async () => {
  const srv = http.createServer((_req, res) => {
    res.writeHead(401, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: "unauthorized" }));
  });
  await new Promise<void>((r) => srv.listen(0, "127.0.0.1", () => r()));
  const addr = srv.address();
  if (!addr || typeof addr === "string") throw new Error("no addr");
  const c = new Client({
    baseUrl: `http://127.0.0.1:${(addr as { port: number }).port}`,
    apiKey: "bad",
  });
  await assert.rejects(() => c.recall({ query: "x" }), (err: unknown) => {
    assert.ok(err instanceof APIError);
    assert.equal(err.status, 401);
    return true;
  });
  await new Promise<void>((r, j) => srv.close((e) => (e ? j(e) : r())));
});
