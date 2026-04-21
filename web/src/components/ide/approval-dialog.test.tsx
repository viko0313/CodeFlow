import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApprovalDialog } from "@/components/ide/approval-dialog";
import { useUiStore } from "@/stores/use-ui-store";

describe("ApprovalDialog", () => {
  afterEach(() => {
    useUiStore.getState().setPendingApproval(undefined);
  });

  it("sends an approval decision", () => {
    useUiStore.getState().setPendingApproval({
      approval_id: "apr_123",
      operation_id: "op_123",
      kind: "shell",
      command: "git status",
      preview: "$ git status",
      risk: "medium",
    });
    const send = vi.fn(() => true);
    render(<ApprovalDialog send={send} />);
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(send).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "permission.decide",
        approval_id: "apr_123",
        operation_id: "op_123",
        allowed: true,
      }),
    );
  });

  it("requires reason when denying", () => {
    useUiStore.getState().setPendingApproval({
      approval_id: "apr_456",
      operation_id: "op_456",
      kind: "write_file",
      preview: "diff",
    });
    const send = vi.fn(() => true);
    render(<ApprovalDialog send={send} />);
    fireEvent.click(screen.getByRole("button", { name: "Deny" }));
    expect(send).not.toHaveBeenCalled();
    expect(screen.getByText("Reject reason is required.")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("Reason required when denying"), {
      target: { value: "unsafe command" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Deny" }));
    expect(send).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "permission.decide",
        approval_id: "apr_456",
        operation_id: "op_456",
        allowed: false,
        reason: "unsafe command",
      }),
    );
  });
});
