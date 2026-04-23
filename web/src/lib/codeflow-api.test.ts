import { afterEach, describe, expect, it, vi } from "vitest";
import { getMcp, getSkills } from "@/lib/codeflow-api";

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
});
