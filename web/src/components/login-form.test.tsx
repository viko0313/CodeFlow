import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LoginForm } from "@/components/login-form";

const push = vi.fn();
const refresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, refresh }),
  useSearchParams: () => new URLSearchParams("callbackUrl=/ide"),
}));

vi.mock("next-auth/react", () => ({
  signIn: vi.fn(async () => ({ ok: true })),
}));

describe("LoginForm", () => {
  it("submits local credentials and redirects to the callback", async () => {
    render(<LoginForm />);
    fireEvent.click(screen.getByRole("button", { name: "进入工作台" }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/ide"));
    expect(refresh).toHaveBeenCalled();
  });
});
