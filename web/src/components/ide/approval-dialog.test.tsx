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
    fireEvent.click(screen.getByRole("button", { name: "批准" }));
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
    fireEvent.click(screen.getByRole("button", { name: "拒绝" }));
    expect(send).not.toHaveBeenCalled();
    expect(screen.getByText("拒绝时必须填写原因。")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("拒绝时请填写原因"), {
      target: { value: "unsafe command" },
    });
    fireEvent.click(screen.getByRole("button", { name: "拒绝" }));
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
