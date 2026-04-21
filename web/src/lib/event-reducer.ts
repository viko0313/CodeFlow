import type { PendingApproval, ServerEvent } from "@/lib/types";

export type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
};

export type EventState = {
  chat: ChatMessage[];
  terminal: string[];
  timeline: ServerEvent[];
  latestDiff?: { path?: string; preview: string };
  pendingApproval?: PendingApproval;
  activeSessionId?: string;
  lastError?: string;
};

export const initialEventState: EventState = {
  chat: [],
  terminal: [],
  timeline: [],
};

export function appendUserMessage(state: EventState, id: string, content: string): EventState {
  return {
    ...state,
    chat: [...state.chat, { id, role: "user", content }],
  };
}

export function reduceServerEvent(state: EventState, event: ServerEvent): EventState {
  const timeline = [event, ...state.timeline].slice(0, 80);
  if (event.type === "chat.token") {
    const id = event.id ?? "assistant";
    const existing = state.chat.find((message) => message.id === id && message.role === "assistant");
    const chat = existing
      ? state.chat.map((message) =>
          message.id === id && message.role === "assistant"
            ? { ...message, content: `${message.content}${event.content ?? ""}` }
            : message,
        )
      : [...state.chat, { id, role: "assistant" as const, content: event.content ?? "" }];
    return { ...state, chat, timeline };
  }
  if (event.type === "chat.output") {
    return {
      ...state,
      chat: [...state.chat, { id: event.id ?? crypto.randomUUID(), role: "assistant", content: event.content ?? "" }],
      timeline,
    };
  }
  if (event.type === "chat.status" || event.type === "chat.stats") {
    return { ...state, timeline };
  }
  if (event.type === "terminal.output") {
    return {
      ...state,
      terminal: [...state.terminal, event.output ?? ""].slice(-100),
      timeline,
    };
  }
  if (event.type === "file.diff") {
    return {
      ...state,
      latestDiff: { path: event.path, preview: event.preview ?? "" },
      timeline,
    };
  }
  if (event.type === "permission.required" && event.operation_id) {
    return {
      ...state,
      pendingApproval: {
        approval_id: event.approval_id,
        operation_id: event.operation_id,
        request_id: event.request_id,
        status: event.status,
        kind: event.kind ?? "operation",
        path: event.path,
        command: event.command,
        preview: event.preview,
        risk: event.risk,
        timeout: event.timeout,
      },
      timeline,
    };
  }
  if (event.type === "approval.updated") {
    if (!state.pendingApproval) {
      return { ...state, timeline };
    }
    if (event.approval_id && state.pendingApproval.approval_id && event.approval_id !== state.pendingApproval.approval_id) {
      return { ...state, timeline };
    }
    if ((event.status ?? "").toLowerCase() === "pending") {
      return {
        ...state,
        pendingApproval: {
          ...state.pendingApproval,
          status: event.status,
        },
        timeline,
      };
    }
    return { ...state, pendingApproval: undefined, timeline };
  }
  if (event.type === "operation.done") {
    return { ...state, pendingApproval: undefined, timeline };
  }
  if (event.type === "operation.error") {
    return { ...state, pendingApproval: undefined, lastError: event.error, timeline };
  }
  if (event.type === "session.updated") {
    return { ...state, activeSessionId: event.session_id, timeline };
  }
  return { ...state, timeline };
}
