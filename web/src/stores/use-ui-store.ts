import { create } from "zustand";
import type { PendingApproval } from "@/lib/types";

type UiState = {
  activeSessionId?: string;
  socketStatus: "offline" | "connecting" | "online";
  pendingApproval?: PendingApproval;
  planEnabled: boolean;
  editorPath: string;
  editorBuffer: string;
  setActiveSessionId: (id?: string) => void;
  setSocketStatus: (status: UiState["socketStatus"]) => void;
  setPendingApproval: (approval?: PendingApproval) => void;
  setPlanEnabled: (enabled: boolean) => void;
  setEditor: (path: string, content: string) => void;
};

export const useUiStore = create<UiState>((set) => ({
  socketStatus: "offline",
  planEnabled: false,
  editorPath: "README.md",
  editorBuffer: "# CodeFlow\n",
  setActiveSessionId: (id) => set({ activeSessionId: id }),
  setSocketStatus: (status) => set({ socketStatus: status }),
  setPendingApproval: (approval) => set({ pendingApproval: approval }),
  setPlanEnabled: (enabled) => set({ planEnabled: enabled }),
  setEditor: (path, content) => set({ editorPath: path, editorBuffer: content }),
}));
