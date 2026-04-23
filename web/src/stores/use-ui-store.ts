import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { initializeEventState, type ChatMessage, type EventState } from "@/lib/event-reducer";
import type { PendingApproval, ServerEvent, SessionHistoryTurn, UploadedDocument } from "@/lib/types";

type PaneSizes = {
  primary: number;
  diffPreviewHeight: number;
  sidebarWidth: number;
};

type SessionIdeState = {
  chat: ChatMessage[];
  terminal: string[];
  timeline: ServerEvent[];
  latestDiff?: { path?: string; preview: string };
  promptDraft: string;
  commandDraft: string;
  editorPath: string;
  editorBuffer: string;
  uploadedDocs: UploadedDocument[];
  paneSizes: PaneSizes;
  showTerminal: boolean;
};

type UiState = {
  activeSessionId?: string;
  socketStatus: "offline" | "connecting" | "online";
  pendingApproval?: PendingApproval;
  planEnabled: boolean;
  ideBySession: Record<string, SessionIdeState>;
  setActiveSessionId: (id?: string) => void;
  setSocketStatus: (status: UiState["socketStatus"]) => void;
  setPendingApproval: (approval?: PendingApproval) => void;
  setPlanEnabled: (enabled: boolean) => void;
  getIdeState: (sessionId?: string) => SessionIdeState;
  setEventState: (sessionId: string, state: EventState) => void;
  hydrateHistory: (sessionId: string, turns: SessionHistoryTurn[]) => void;
  setPromptDraft: (sessionId: string, promptDraft: string) => void;
  setCommandDraft: (sessionId: string, commandDraft: string) => void;
  setEditor: (sessionId: string, path: string, content: string) => void;
  setLatestDiff: (sessionId: string, latestDiff?: { path?: string; preview: string }) => void;
  addUploadedDocument: (sessionId: string, doc: UploadedDocument) => void;
  setPaneSizes: (sessionId: string, paneSizes: PaneSizes) => void;
  toggleTerminal: (sessionId: string) => void;
  resetForTests: () => void;
};

const defaultPaneSizes: PaneSizes = {
  primary: 56,
  diffPreviewHeight: 180,
  sidebarWidth: 280,
};

function createDefaultSessionIdeState(): SessionIdeState {
  return {
    chat: [],
    terminal: [],
    timeline: [],
    latestDiff: undefined,
    promptDraft: "",
    commandDraft: "git status --short",
    editorPath: "README.md",
    editorBuffer: "# CodeFlow\n",
    uploadedDocs: [],
    paneSizes: { ...defaultPaneSizes },
    showTerminal: false,
  };
}

function normalizeSessionState(session: Partial<SessionIdeState> | undefined): SessionIdeState {
  const base = createDefaultSessionIdeState();
  return {
    ...base,
    ...session,
    paneSizes: {
      ...base.paneSizes,
      ...(session?.paneSizes ?? {}),
    },
    uploadedDocs: session?.uploadedDocs ?? base.uploadedDocs,
    chat: session?.chat ?? base.chat,
    terminal: session?.terminal ?? base.terminal,
    timeline: session?.timeline ?? base.timeline,
    showTerminal: session?.showTerminal ?? base.showTerminal,
  };
}

function resolveSessionId(sessionId?: string, activeSessionId?: string) {
  return sessionId?.trim() || activeSessionId?.trim() || "default";
}

function mergeEventState(current: SessionIdeState, state: EventState): SessionIdeState {
  return {
    ...current,
    chat: state.chat,
    terminal: state.terminal,
    timeline: state.timeline,
    latestDiff: state.latestDiff,
  };
}

const initialUiState = {
  activeSessionId: undefined,
  socketStatus: "offline" as const,
  pendingApproval: undefined,
  planEnabled: false,
  ideBySession: {} as Record<string, SessionIdeState>,
};

export const useUiStore = create<UiState>()(
  persist(
    (set, get) => ({
      ...initialUiState,
      setActiveSessionId: (id) => set({ activeSessionId: id }),
      setSocketStatus: (status) => set({ socketStatus: status }),
      setPendingApproval: (approval) => set({ pendingApproval: approval }),
      setPlanEnabled: (enabled) => set({ planEnabled: enabled }),
      getIdeState: (sessionId) => {
        const key = resolveSessionId(sessionId, get().activeSessionId);
        return normalizeSessionState(get().ideBySession[key]);
      },
      setEventState: (sessionId, state) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: mergeEventState(session, state),
            },
          };
        }),
      hydrateHistory: (sessionId, turns) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          if (session.chat.length > 0 || turns.length === 0) {
            return current;
          }
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: mergeEventState(session, initializeEventState(turns)),
            },
          };
        }),
      setPromptDraft: (sessionId, promptDraft) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: { ...session, promptDraft },
            },
          };
        }),
      setCommandDraft: (sessionId, commandDraft) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: { ...session, commandDraft },
            },
          };
        }),
      setEditor: (sessionId, path, content) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: { ...session, editorPath: path, editorBuffer: content },
            },
          };
        }),
      setLatestDiff: (sessionId, latestDiff) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: { ...session, latestDiff },
            },
          };
        }),
      addUploadedDocument: (sessionId, doc) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: {
                ...session,
                uploadedDocs: [doc, ...session.uploadedDocs.filter((item) => item.id !== doc.id)].slice(0, 5),
              },
            },
          };
        }),
      setPaneSizes: (sessionId, paneSizes) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = current.ideBySession[key] ?? createDefaultSessionIdeState();
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: { ...session, paneSizes: { ...paneSizes } },
            },
          };
        }),
      toggleTerminal: (sessionId) =>
        set((current) => {
          const key = resolveSessionId(sessionId, current.activeSessionId);
          const session = normalizeSessionState(current.ideBySession[key]);
          return {
            ideBySession: {
              ...current.ideBySession,
              [key]: { ...session, showTerminal: !session.showTerminal },
            },
          };
        }),
      resetForTests: () => {
        set({
          ...initialUiState,
          ideBySession: {},
        });
      },
    }),
    {
      name: "codeflow-ui",
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({
        activeSessionId: state.activeSessionId,
        planEnabled: state.planEnabled,
        ideBySession: state.ideBySession,
      }),
    },
  ),
);
