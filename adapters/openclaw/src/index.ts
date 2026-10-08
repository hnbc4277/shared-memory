/**
 * OpenClaw (xiaolongxia) plugin — thin Shared Memory SDK bridge.
 */

import { Client } from "@sharedmemory/sdk";
import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";

const AGENT = "xiaolongxia";

type PluginConfig = {
  apiUrl?: string;
  apiKey?: string;
};

function makeClient(cfg: PluginConfig = {}): Client {
  return new Client({
    baseUrl: cfg.apiUrl || process.env.SHARED_MEMORY_API_URL,
    apiKey: cfg.apiKey || process.env.SHARED_MEMORY_API_KEY,
  });
}

function asText(v: unknown): string {
  if (typeof v === "string") return v;
  if (v == null) return "";
  try {
    return JSON.stringify(v);
  } catch {
    return String(v);
  }
}

function sessionKeyOf(event: any, ctx: any): string {
  return (
    asText(event?.sessionKey) ||
    asText(ctx?.sessionKey) ||
    asText(event?.sessionId) ||
    asText(ctx?.sessionId) ||
    ""
  );
}

function subIdFromSpawn(event: any): string {
  return (
    asText(event?.agentId) ||
    asText(event?.subagentId) ||
    asText(event?.label) ||
    asText(event?.childSessionKey) ||
    "sub"
  );
}

function asStringList(v: unknown): string[] {
  if (!Array.isArray(v)) return [];
  return v.map((x) => String(x ?? "").trim()).filter(Boolean);
}

function provenanceOf(event: any): { skills: string[]; plugins: string[]; tools: string[]; model: string } {
  return {
    skills: asStringList(event?.skills ?? event?.skill),
    plugins: asStringList(event?.plugins ?? event?.plugin),
    tools: asStringList(event?.tools ?? event?.tool),
    model: asText(event?.model || event?.modelName).trim() || "unknown",
  };
}

async function ingestSafe(
  client: Client,
  fields: {
    role: string;
    content: string;
    external_session_id?: string;
    agent_path?: string;
    skills?: string[];
    plugins?: string[];
    tools?: string[];
    model?: string;
  },
): Promise<void> {
  const content = fields.content.trim();
  if (!content) return;
  try {
    await client.ingest({
      agent: AGENT,
      agent_path: fields.agent_path || AGENT,
      role: fields.role,
      content,
      external_session_id: fields.external_session_id || undefined,
      skills: fields.skills ?? [],
      plugins: fields.plugins ?? [],
      tools: fields.tools ?? [],
      model: fields.model || "unknown",
    });
  } catch {
    // Observation hooks must not break the host.
  }
}

export default definePluginEntry({
  id: "shared-memory",
  name: "Shared Memory",
  description: "Ingest/recall via Shared Memory for OpenClaw (xiaolongxia)",
  register(api) {
    const client = makeClient();
    /** childSessionKey → agent_path suffix id */
    const childMap = new Map<string, string>();

    api.on("before_prompt_build", async (event, ctx) => {
      const query =
        asText(event?.prompt) ||
        asText(event?.message) ||
        asText(event?.text) ||
        "session";
      try {
        const recalled = await client.recall({ query });
        const context = asText(recalled?.context).trim();
        if (!context) return;
        return { prependContext: context };
      } catch {
        return;
      }
    });

    api.on("message_received", async (event, ctx) => {
      const content =
        asText(event?.content) ||
        asText(event?.text) ||
        asText(event?.body) ||
        asText(event?.message);
      const sessionKey = sessionKeyOf(event, ctx);
      const childId = childMap.get(sessionKey);
      const prov = provenanceOf(event);
      await ingestSafe(client, {
        role: "user",
        content,
        external_session_id: sessionKey || undefined,
        agent_path: childId ? `${AGENT}/${childId}` : AGENT,
        ...prov,
      });
    });

    api.on("agent_end", async (event, ctx) => {
      const messages = Array.isArray(event?.messages) ? event.messages : [];
      let assistant = "";
      for (let i = messages.length - 1; i >= 0; i--) {
        const m = messages[i];
        const role = asText(m?.role).toLowerCase();
        if (role === "assistant" || role === "model") {
          assistant =
            asText(m?.content) || asText(m?.text) || asText(m?.body);
          if (assistant) break;
        }
      }
      if (!assistant) {
        assistant =
          asText(event?.text) ||
          asText(event?.response) ||
          asText(event?.finalText) ||
          asText(event?.message);
      }
      const sessionKey = sessionKeyOf(event, ctx);
      const childId = childMap.get(sessionKey);
      const prov = provenanceOf(event);
      await ingestSafe(client, {
        role: "assistant",
        content: assistant,
        external_session_id: sessionKey || undefined,
        agent_path: childId ? `${AGENT}/${childId}` : AGENT,
        ...prov,
      });
    });

    api.on("subagent_spawned", async (event) => {
      const childKey = asText(event?.childSessionKey);
      const id = subIdFromSpawn(event);
      if (childKey) {
        childMap.set(childKey, id);
      }
    });

    api.on("subagent_ended", async (event) => {
      const target = asText(event?.targetSessionKey);
      if (target) {
        childMap.delete(target);
      }
    });
  },
});
