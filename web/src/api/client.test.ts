import { describe, expect, it } from "vitest";
import { ApiError, buildUrl, createClient } from "./client";

describe("buildUrl", () => {
  it("joins base and path without double slashes", () => {
    expect(buildUrl("http://h:1/", "/v1/sessions")).toBe("http://h:1/v1/sessions");
  });

  it("drops empty query values and encodes the rest", () => {
    expect(buildUrl("http://h:1", "v1/sessions", { agent_path: "xiaolongxia/researcher", project_id: "", limit: 5, x: undefined }))
      .toBe("http://h:1/v1/sessions?agent_path=xiaolongxia%2Fresearcher&limit=5");
  });
});

describe("createClient", () => {
  it("sends the bearer key and surfaces API errors", async () => {
    let seen: RequestInit | undefined;
    const fake = (async (_url: string, init?: RequestInit) => {
      seen = init;
      return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
    }) as unknown as typeof fetch;
    const c = createClient("http://h:1", () => "sm_key", fake);
    const err = (await c.get("/v1/me").catch((e: unknown) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(401);
    expect(err.message).toBe("未授权，请重新登录");
    expect((seen?.headers as Record<string, string>)["Authorization"]).toBe("Bearer sm_key");
  });
});
