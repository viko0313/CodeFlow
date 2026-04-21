import { describe, expect, it } from "vitest";
import { initialEventState, reduceServerEvent } from "@/lib/event-reducer";

describe("reduceServerEvent", () => {
  it("accumulates streamed assistant tokens", () => {
    const first = reduceServerEvent(initialEventState, {
      type: "chat.token",
      id: "m1",
      content: "Hel",
    });
    const second = reduceServerEvent(first, {
      type: "chat.token",
      id: "m1",
      content: "lo",
    });
    expect(second.chat).toEqual([{ id: "m1", role: "assistant", content: "Hello" }]);
  });

  it("opens and clears permission approvals", () => {
    const pending = reduceServerEvent(initialEventState, {
      type: "permission.required",
      approval_id: "apr_1",
      operation_id: "op_1",
      kind: "shell",
      command: "git status",
      preview: "$ git status",
    });
    expect(pending.pendingApproval?.approval_id).toBe("apr_1");
    expect(pending.pendingApproval?.operation_id).toBe("op_1");
    const done = reduceServerEvent(pending, { type: "operation.done", id: "run1" });
    expect(done.pendingApproval).toBeUndefined();
  });

  it("clears pending approval after approval.updated", () => {
    const pending = reduceServerEvent(initialEventState, {
      type: "permission.required",
      approval_id: "apr_1",
      operation_id: "op_1",
      kind: "shell",
    });
    const decided = reduceServerEvent(pending, {
      type: "approval.updated",
      approval_id: "apr_1",
      status: "approved",
    });
    expect(decided.pendingApproval).toBeUndefined();
  });
});
