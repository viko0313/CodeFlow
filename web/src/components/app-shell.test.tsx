import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "@/components/app-shell";

vi.mock("@/auth", () => ({
  signOut: async () => undefined,
}));

describe("AppShell", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("preserves the current ide session in navigation links", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    render(
      <AppShell active="dashboard" activeSessionId="session-42">
        <div>content</div>
      </AppShell>,
    );

    expect(screen.getByRole("link", { name: "IDE" })).toHaveAttribute("href", "/ide?session=session-42");
  });
});
