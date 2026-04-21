"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  appendUserMessage,
  initialEventState,
  reduceServerEvent,
  type EventState,
} from "@/lib/event-reducer";
import type { ClientMessage, ServerEvent } from "@/lib/types";
import { useUiStore } from "@/stores/use-ui-store";

const wsBase = process.env.NEXT_PUBLIC_CODEFLOW_WS_URL ?? "ws://localhost:8742/api/ws";

export function useCodeFlowSocket(sessionId?: string) {
  const socketRef = useRef<WebSocket | null>(null);
  const [state, setState] = useState<EventState>(initialEventState);
  const setSocketStatus = useUiStore((store) => store.setSocketStatus);
  const setPendingApproval = useUiStore((store) => store.setPendingApproval);
  const setActiveSessionId = useUiStore((store) => store.setActiveSessionId);
  const planEnabled = useUiStore((store) => store.planEnabled);

  useEffect(() => {
    if (!sessionId) return;
    setSocketStatus("connecting");
    const url = new URL(wsBase);
    url.searchParams.set("session_id", sessionId);
    const socket = new WebSocket(url);
    socketRef.current = socket;
    socket.onopen = () => setSocketStatus("online");
    socket.onclose = () => setSocketStatus("offline");
    socket.onerror = () => setSocketStatus("offline");
    socket.onmessage = (message) => {
      const event = JSON.parse(message.data as string) as ServerEvent;
      setState((current) => {
        const next = reduceServerEvent(current, event);
        if (next.pendingApproval !== current.pendingApproval) {
          setPendingApproval(next.pendingApproval);
        }
        if (next.activeSessionId && next.activeSessionId !== current.activeSessionId) {
          setActiveSessionId(next.activeSessionId);
        }
        return next;
      });
    };
    return () => {
      socket.close();
      if (socketRef.current === socket) {
        socketRef.current = null;
      }
    };
  }, [sessionId, setActiveSessionId, setPendingApproval, setSocketStatus]);

  const send = useCallback((message: ClientMessage) => {
    if (socketRef.current?.readyState !== WebSocket.OPEN) return false;
    const payload: ClientMessage = {
      request_id: message.request_id ?? crypto.randomUUID(),
      ...message,
    };
    socketRef.current.send(JSON.stringify(payload));
    return true;
  }, []);

  const sendChat = useCallback(
    (input: string) => {
      const id = crypto.randomUUID();
      setState((current) => appendUserMessage(current, id, input));
      return send({ type: "chat.send", id, input, plan_enabled: planEnabled });
    },
    [planEnabled, send],
  );

  return useMemo(
    () => ({
      state,
      send,
      sendChat,
    }),
    [send, sendChat, state],
  );
}
