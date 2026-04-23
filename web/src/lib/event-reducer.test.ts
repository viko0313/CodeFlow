import { describe, expect, it } from "vitest";
import { initialEventState, initializeEventState, reduceServerEvent } from "@/lib/event-reducer";

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

  it("hydrates recent history before realtime events", () => {
    const hydrated = initializeEventState([
      { role: "user", content: "Check README", created_at: "2026-04-22T00:00:00Z" },
      { role: "assistant", content: "Sure.", created_at: "2026-04-22T00:00:01Z" },
    ]);

    expect(hydrated.chat).toEqual([
      { id: "history-0", role: "user", content: "Check README" },
      { id: "history-1", role: "assistant", content: "Sure." },
    ]);

    const next = reduceServerEvent(hydrated, {
      type: "chat.token",
      id: "live-1",
      content: "New",
    });

    expect(next.chat).toEqual([
      { id: "history-0", role: "user", content: "Check README" },
      { id: "history-1", role: "assistant", content: "Sure." },
      { id: "live-1", role: "assistant", content: "New" },
    ]);
  });
});
