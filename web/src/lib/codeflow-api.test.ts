import { afterEach, describe, expect, it, vi } from "vitest";
import { getMcp, getSkills, updateModelConfig } from "@/lib/codeflow-api";

describe("codeflow api normalization", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("normalizes null skills to an empty array", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ enabled: true, dirs: [], skills: null }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(getSkills()).resolves.toMatchObject({ skills: [] });
  });

  it("normalizes null mcp servers to an empty array", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ enabled: true, preload: true, config_files: [], servers: null }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(getMcp()).resolves.toMatchObject({ servers: [] });
  });

  it("updates model config with a PUT payload", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          provider: "openai",
          model: "gpt-4.1",
          base_url: "https://api.openai.com/v1",
          api_key_configured: true,
          api_key_hint: "sk-...1234",
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      ),
    );

    await updateModelConfig({
      provider: "openai",
      model: "gpt-4.1",
      base_url: "https://api.openai.com/v1",
      api_key: "sk-test-secret-1234",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/codeflow/config/model",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          provider: "openai",
          model: "gpt-4.1",
          base_url: "https://api.openai.com/v1",
          api_key: "sk-test-secret-1234",
        }),
      }),
    );
  });
});
