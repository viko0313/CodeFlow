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
      operation_id: "op_123",
      kind: "shell",
      command: "git status",
      preview: "$ git status",
      risk: "medium",
    });
    const send = vi.fn(() => true);
    render(<ApprovalDialog send={send} />);
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(send).toHaveBeenCalledWith({
      type: "permission.decide",
      operation_id: "op_123",
      allowed: true,
    });
  });
});
