import { describe, expect, it } from "vitest";
import { formatDateTime } from "@/lib/utils";

describe("formatDateTime", () => {
  it("returns a stable UTC timestamp string", () => {
    expect(formatDateTime("2026-04-21T17:27:24Z")).toBe("2026-04-21 17:27 UTC");
  });
});
