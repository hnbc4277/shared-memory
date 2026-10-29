/**
 * Seed demo data via API for manual screenshots (requires API on MANUAL_API_URL).
 */
const API = (process.env.MANUAL_API_URL ?? "http://127.0.0.1:18080").replace(/\/+$/, "");

async function sm(method, path, body, key) {
  const headers = { "Content-Type": "application/json; charset=utf-8" };
  if (key) headers.Authorization = `Bearer ${key}`;
  const res = await fetch(`${API}${path}`, {
    method,
    headers,
    body: body != null ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = null;
  }
  return { status: res.status, json, text };
}

function msg(projectId, role, content, extra = {}) {
  return {
    project_id: projectId,
    agent: "xiaolongxia",
    agent_path: "xiaolongxia/researcher",
    skills: ["deep-research"],
    plugins: ["browser"],
    tools: ["web_fetch"],
    model: "deepseek-v3",
    role,
    content,
    ...extra,
  };
}

export async function seedAccount() {
  const health = await sm("GET", "/healthz");
  if (health.status !== 200) {
    throw new Error(`API 不可用 (${API}/healthz → HTTP ${health.status})。请先 docker compose --profile app up -d --build`);
  }

  const email = `manual-shot-${Date.now()}@example.com`;
  const password = "password12345";
  const reg = await sm("POST", "/v1/auth/register", { email, password });
  if (reg.status !== 201 || !reg.json?.api_key) {
    throw new Error(`注册失败 HTTP ${reg.status}: ${reg.text}`);
  }
  const apiKey = reg.json.api_key;

  const proj = await sm("POST", "/v1/projects", { name: "手册演示项目" }, apiKey);
  if (proj.status !== 201) throw new Error(`创建项目失败 HTTP ${proj.status}`);
  const projectId = proj.json.id;

  const marker = `manual-${Date.now().toString(36)}`;
  await sm("POST", "/v1/ingest", msg(projectId, "user", `我们决定采用 OpenSearch 做全文检索 ${marker}`), apiKey);
  await sm("POST", "/v1/ingest", msg(projectId, "assistant", `已记录：OpenSearch 用于检索层 ${marker}`), apiKey);

  const conn = await sm(
    "POST",
    "/v1/agents/connections",
    { agent: "xiaolongxia", label: "手册演示连接", issue_dedicated_key: false },
    apiKey,
  );
  const connectionId = conn.json?.connection_id ?? null;
  if (connectionId) {
    await sm(
      "POST",
      "/v1/ingest",
      msg(projectId, "user", `绑定 connection 的会话 ${marker}`, { connection_id: connectionId }),
      apiKey,
    );
  }

  await sm("POST", "/v1/recall", { query: marker, top_k: 5 }, apiKey);

  const secret = `Demo!${Date.now().toString(36)}`;
  const vaultIngest = await sm(
    "POST",
    "/v1/ingest",
    msg(projectId, "user", `GitLab https://gitlab.example.com 账号: demo 密码: ${secret}`),
    apiKey,
  );
  const vaultPrompt = vaultIngest.json?.prompts?.find((p) => p.kind === "vault");
  const vaultCandidateId = vaultPrompt?.candidate_id ?? null;
  const secret2 = `Saved!${Date.now().toString(36)}`;
  const vaultIngest2 = await sm(
    "POST",
    "/v1/ingest",
    msg(projectId, "user", `内部 wiki 账号: ops 密码: ${secret2}`),
    apiKey,
  );
  const vaultPrompt2 = vaultIngest2.json?.prompts?.find((p) => p.kind === "vault");
  if (vaultPrompt2?.candidate_id) {
    await sm("POST", `/v1/vault/candidates/${vaultPrompt2.candidate_id}/accept`, {}, apiKey);
  }

  const long = `# 手册演示成稿\n${"消费者组与 XAUTOCLAIM 用法说明。".repeat(40)}`;
  const artIngest = await sm("POST", "/v1/ingest", msg(projectId, "assistant", long), apiKey);
  const artPrompt = artIngest.json?.prompts?.find((p) => p.kind === "artifact");
  const artifactCandidateId = artPrompt?.candidate_id ?? null;

  return {
    apiKey,
    email,
    password,
    projectId,
    projectName: "手册演示项目",
    connectionId,
    marker,
    vaultCandidateId,
    artifactCandidateId,
  };
}
