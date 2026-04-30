"use client";

import dynamic from "next/dynamic";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileUp, GripVertical, PanelBottomClose, PanelBottomOpen, Send, TerminalSquare, Wand2 } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect } from "react";
import { ApprovalDialog } from "@/components/ide/approval-dialog";
import { TerminalPanel } from "@/components/ide/terminal-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useCodeFlowSocket } from "@/hooks/use-codeflow-socket";
import { getConfig, getSessionHistory, getSessions, switchSession, uploadDocument } from "@/lib/codeflow-api";
import type { UploadedDocument } from "@/lib/types";
import { useUiStore } from "@/stores/use-ui-store";

const MonacoEditor = dynamic(() => import("@monaco-editor/react"), { ssr: false });

const fallbackIdeState = {
  chat: [],
  terminal: [],
  timeline: [],
  latestDiff: undefined,
  promptDraft: "",
  commandDraft: "git status --short",
  editorPath: "README.md",
  editorBuffer: "# CodeFlow\n",
  uploadedDocs: [] as UploadedDocument[],
  paneSizes: { primary: 56, diffPreviewHeight: 180, sidebarWidth: 280 },
  showTerminal: false,
};

export function IdeClient() {
  const params = useSearchParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: getSessions });
  const config = useQuery({ queryKey: ["config"], queryFn: getConfig });
  const activeFromStore = useUiStore((store) => store.activeSessionId);
  const setActiveSessionId = useUiStore((store) => store.setActiveSessionId);
  const socketStatus = useUiStore((store) => store.socketStatus);
  const planEnabled = useUiStore((store) => store.planEnabled);
  const setPlanEnabled = useUiStore((store) => store.setPlanEnabled);
  const hydrateHistory = useUiStore((store) => store.hydrateHistory);
  const setPromptDraft = useUiStore((store) => store.setPromptDraft);
  const setCommandDraft = useUiStore((store) => store.setCommandDraft);
  const setEditor = useUiStore((store) => store.setEditor);
  const addUploadedDocument = useUiStore((store) => store.addUploadedDocument);
  const setPaneSizes = useUiStore((store) => store.setPaneSizes);
  const toggleTerminal = useUiStore((store) => store.toggleTerminal);

  const initialSession =
    params.get("session") ?? sessions.data?.find((session) => session.active)?.id ?? sessions.data?.[0]?.id;
  const activeSessionId = params.get("session") ?? activeFromStore ?? initialSession;
  const persistedSessionState = useUiStore((store) => (activeSessionId ? store.ideBySession[activeSessionId] : undefined));
  const sessionState = persistedSessionState ?? fallbackIdeState;
  const { state, send, sendChat, restoreState } = useCodeFlowSocket(activeSessionId);
  const history = useQuery({
    queryKey: ["session-history", activeSessionId],
    queryFn: () => getSessionHistory(activeSessionId!, { limit: 20 }),
    enabled: Boolean(activeSessionId),
  });

  useEffect(() => {
    if (initialSession && activeFromStore !== initialSession) {
      setActiveSessionId(initialSession);
    }
  }, [activeFromStore, initialSession, setActiveSessionId]);

  useEffect(() => {
    if (activeSessionId && params.get("session") !== activeSessionId) {
      router.replace(`/ide?session=${encodeURIComponent(activeSessionId)}`);
    }
  }, [activeSessionId, params, router]);

  useEffect(() => {
    if (config.data?.agent) {
      setPlanEnabled(config.data.agent.plan_enabled);
    }
  }, [config.data?.agent, setPlanEnabled]);

  useEffect(() => {
    if (activeSessionId && history.data?.turns) {
      hydrateHistory(activeSessionId, history.data.turns);
      restoreState(useUiStore.getState().getIdeState(activeSessionId));
    }
  }, [activeSessionId, history.data?.turns, hydrateHistory, restoreState]);

  const switchMutation = useMutation({
    mutationFn: switchSession,
    onSuccess: (session) => {
      setActiveSessionId(session.id);
      router.replace(`/ide?session=${encodeURIComponent(session.id)}`);
      send({ type: "session.switch", session_id: session.id });
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });

  const activeTitle = sessions.data?.find((session) => session.id === activeSessionId)?.title ?? "工作台";

  function submitPrompt(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!activeSessionId) return;
    const value = sessionState.promptDraft.trim();
    if (!value) return;
    if (sendChat(value)) {
      setPromptDraft(activeSessionId, "");
    }
  }

  function runCommand(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!activeSessionId) return;
    const command = sessionState.commandDraft.trim();
    if (!command) return;
    send({ type: "terminal.run", id: crypto.randomUUID(), command });
  }

  function previewFile() {
    if (!activeSessionId) return;
    send({
      type: "file.preview",
      id: crypto.randomUUID(),
      path: sessionState.editorPath,
      content: sessionState.editorBuffer,
    });
  }

  function writeFile() {
    if (!activeSessionId) return;
    send({
      type: "file.write",
      id: crypto.randomUUID(),
      path: sessionState.editorPath,
      content: sessionState.editorBuffer,
    });
  }

  async function uploadSelectedFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file || !activeSessionId) return;
    const doc = await uploadDocument(file);
    addUploadedDocument(activeSessionId, doc);
    event.target.value = "";
  }

  function startSidebarResize(event: React.PointerEvent<HTMLDivElement>) {
    if (!activeSessionId) return;
    const startX = event.clientX;
    const start = sessionState.paneSizes.sidebarWidth;
    const handleMove = (moveEvent: PointerEvent) => {
      const delta = moveEvent.clientX - startX;
      setPaneSizes(activeSessionId, {
        ...sessionState.paneSizes,
        sidebarWidth: clamp(start + delta, 240, 520),
      });
    };
    const handleUp = () => {
      window.removeEventListener("pointermove", handleMove);
      window.removeEventListener("pointerup", handleUp);
    };
    window.addEventListener("pointermove", handleMove);
    window.addEventListener("pointerup", handleUp);
  }

  function startDiffResize(event: React.PointerEvent<HTMLDivElement>) {
    if (!activeSessionId || window.innerWidth < 1024) return;
    const startY = event.clientY;
    const start = sessionState.paneSizes.diffPreviewHeight;
    const handleMove = (moveEvent: PointerEvent) => {
      const delta = startY - moveEvent.clientY;
      setPaneSizes(activeSessionId, {
        ...sessionState.paneSizes,
        diffPreviewHeight: clamp(start + delta, 160, 340),
      });
    };
    const handleUp = () => {
      window.removeEventListener("pointermove", handleMove);
      window.removeEventListener("pointerup", handleUp);
    };
    window.addEventListener("pointermove", handleMove);
    window.addEventListener("pointerup", handleUp);
  }

  const rightPanelStyle =
    typeof window !== "undefined" && window.innerWidth >= 1024
      ? {
          gridTemplateRows: `auto minmax(0, 1fr) 10px ${sessionState.paneSizes.diffPreviewHeight}px`,
        }
      : undefined;

  return (
    <div
      className="grid h-[calc(100vh-66px)] min-h-[760px] grid-cols-1 overflow-hidden lg:grid-cols-[var(--sidebar-width)_10px_minmax(0,1fr)]"
      style={{ ["--sidebar-width" as string]: `${sessionState.paneSizes.sidebarWidth}px` }}
    >
      <aside className="border-b border-[var(--line)] bg-white p-4 lg:border-b-0 lg:border-r">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-semibold">会话</h2>
          <Badge className={socketStatus === "online" ? "border-[var(--accent)] text-[var(--accent-strong)]" : ""}>
            {socketStatus === "online" ? "在线" : socketStatus === "connecting" ? "连接中" : "离线"}
          </Badge>
        </div>
        <div className="grid max-h-72 gap-2 overflow-auto lg:max-h-[calc(100vh-160px)]">
          {(sessions.data ?? []).map((session) => (
            <button
              key={session.id}
              className={`rounded-xl border px-3 py-3 text-left text-sm transition-colors ${
                session.id === activeSessionId
                  ? "border-[var(--accent)] bg-[var(--panel-strong)]"
                  : "border-[var(--line)] bg-white hover:bg-[var(--panel-strong)]"
              }`}
              onClick={() => switchMutation.mutate(session.id)}
            >
              <span className="block truncate font-medium">{session.title}</span>
              <span className="mt-1 block truncate text-xs text-[var(--muted)]">{session.id}</span>
            </button>
          ))}
        </div>
      </aside>

      <div
        className="hidden cursor-col-resize items-center justify-center border-r border-[var(--line)] bg-[var(--background)] lg:flex"
        onPointerDown={startSidebarResize}
        role="separator"
        aria-label="Resize session sidebar"
      >
        <GripVertical className="h-4 w-4 text-[var(--muted)]" />
      </div>

      <section className="grid min-h-0 grid-rows-[auto_1fr]">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] bg-white px-4 py-3">
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold">{activeTitle}</h1>
            <p className="truncate text-xs text-[var(--muted)]">
              {activeSessionId ?? "当前没有活动会话"} / agent {config.data?.agent?.mode ?? "react"}
            </p>
            <p className="mt-1 truncate text-xs text-[var(--muted)]">项目根：{config.data?.project_root ?? "未知"}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <label className="inline-flex h-9 cursor-pointer items-center gap-2 rounded-lg border border-[var(--line)] bg-white px-3 text-xs font-medium hover:bg-[var(--panel-strong)]">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[var(--accent)]"
                checked={planEnabled}
                onChange={(event) => setPlanEnabled(event.target.checked)}
              />
              计划模式
            </label>
            <label className="inline-flex h-9 cursor-pointer items-center gap-2 rounded-lg border border-[var(--line)] bg-white px-3 text-xs font-medium hover:bg-[var(--panel-strong)]">
              <FileUp className="h-4 w-4" />
              上传文档
              <input className="hidden" type="file" onChange={uploadSelectedFile} />
            </label>
            <Button variant="secondary" size="sm" className="h-9 px-4 text-sm" onClick={previewFile}>
              预览差异
            </Button>
            <Button size="sm" className="h-9 px-4 text-sm" onClick={writeFile}>
              写入文件
            </Button>
            <Button variant="secondary" size="sm" className="h-9 px-4 text-sm" onClick={() => activeSessionId && toggleTerminal(activeSessionId)}>
              {sessionState.showTerminal ? <PanelBottomClose className="h-4 w-4" /> : <PanelBottomOpen className="h-4 w-4" />}
              {sessionState.showTerminal ? "隐藏终端" : "终端"}
            </Button>
          </div>
        </div>

        <div className="grid min-h-0 gap-0">
          <div className={`grid min-h-0 ${sessionState.showTerminal ? "grid-rows-[minmax(0,1fr)_auto_320px]" : "grid-rows-[minmax(0,1fr)_auto]"}`}>
            <div className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] bg-white">
              <div className="overflow-auto p-4">
                <div className="grid gap-3">
                  {state.chat.map((message) => (
                    <div
                      key={`${message.role}-${message.id}-${message.content.length}`}
                      className={`rounded-xl border border-[var(--line)] p-4 text-sm leading-6 ${
                        message.role === "user" ? "bg-[var(--panel-strong)]" : "bg-white"
                      }`}
                    >
                      <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--muted)]">
                        {getRoleLabel(message.role)}
                      </p>
                      {message.status ? <p className="mb-2 text-xs text-[var(--accent-strong)]">{message.status}</p> : null}
                      <p className="whitespace-pre-wrap break-words">{message.content}</p>
                    </div>
                  ))}
                  {!state.chat.length ? (
                    <p className="rounded-xl border border-dashed border-[var(--line)] p-5 text-sm text-[var(--muted)]">
                      可以向 CodeFlow 提问、解释代码，或者规划接下来的修改。
                    </p>
                  ) : null}
                </div>
              </div>

              <form className="border-t border-[var(--line)] p-3" onSubmit={submitPrompt}>
                {sessionState.uploadedDocs.length ? (
                  <div className="mb-3 grid gap-2">
                    {sessionState.uploadedDocs.map((doc) => (
                      <div key={doc.id} className="rounded-lg border border-[var(--line)] bg-[var(--panel-strong)] px-3 py-2">
                        <div className="flex flex-wrap items-center gap-2 text-xs">
                          <Badge>{doc.file_name}</Badge>
                          <span className="text-[var(--muted)]">{doc.chunks} 个分块</span>
                        </div>
                        <p className="mt-1 text-xs text-[var(--muted)]">{getDocumentStatus(doc)}</p>
                      </div>
                    ))}
                  </div>
                ) : null}
                <div className="flex gap-2">
                  <Input
                    value={sessionState.promptDraft}
                    onChange={(event) => activeSessionId && setPromptDraft(activeSessionId, event.target.value)}
                    placeholder="向 CodeFlow 提问"
                    className="h-11"
                  />
                  <Button type="submit" className="h-11 min-w-28 px-4 text-sm">
                    <Send className="h-4 w-4" />
                    发送
                  </Button>
                </div>
              </form>
            </div>

            {sessionState.showTerminal ? (
              <div className="grid min-h-0 grid-rows-[auto_1fr] border-t border-[var(--line)] bg-[var(--terminal)]">
                <form className="flex gap-2 border-b border-white/10 p-3" onSubmit={runCommand}>
                  <Input
                    className="h-11 border-white/20 bg-[#24282c] text-white placeholder:text-white/40"
                    value={sessionState.commandDraft}
                    onChange={(event) => activeSessionId && setCommandDraft(activeSessionId, event.target.value)}
                  />
                  <Button type="submit" variant="secondary" className="h-11 min-w-28 border-white/20 bg-white/10 px-4 text-sm text-white hover:bg-white/16">
                    <TerminalSquare className="h-4 w-4" />
                    运行
                  </Button>
                </form>
                <TerminalPanel output={state.terminal.join("\r\n")} />
              </div>
            ) : null}
          </div>

          <div className="grid min-h-0 bg-white" style={rightPanelStyle}>
            <div className="flex flex-wrap items-center gap-2 border-b border-[var(--line)] p-3">
              <Input
                className="max-w-md"
                value={sessionState.editorPath}
                onChange={(event) => activeSessionId && setEditor(activeSessionId, event.target.value, sessionState.editorBuffer)}
              />
              <Badge>{state.latestDiff?.path ?? "暂无差异"}</Badge>
            </div>
            <MonacoEditor
              height="100%"
              language="markdown"
              path={sessionState.editorPath}
              value={sessionState.editorBuffer}
              theme="vs-light"
              options={{ minimap: { enabled: false }, fontSize: 14, wordWrap: "on" }}
              onChange={(value) => activeSessionId && setEditor(activeSessionId, sessionState.editorPath, value ?? "")}
            />
            <div
              className="hidden cursor-row-resize items-center justify-center border-t border-[var(--line)] bg-[var(--background)] lg:flex"
              onPointerDown={startDiffResize}
              role="separator"
              aria-label="Resize bottom panel"
            >
              <GripVertical className="h-4 w-4 rotate-90 text-[var(--muted)]" />
            </div>
            <div className="grid min-h-0 border-t border-[var(--line)] md:grid-cols-[1.2fr_320px]">
              <div className="min-h-0 p-3">
                <div className="mb-2 flex items-center gap-2">
                  <Wand2 className="h-4 w-4 text-[var(--accent-strong)]" />
                  <h2 className="text-sm font-semibold">差异预览</h2>
                </div>
                <Textarea
                  readOnly
                  className="h-full min-h-[160px] resize-y font-mono text-[11px] leading-5"
                  style={{ height: sessionState.paneSizes.diffPreviewHeight - 52 }}
                  value={state.latestDiff?.preview ?? "预览或写入文件后，这里会显示差异内容。"}
                />
              </div>
              <div className="min-h-0 border-t border-[var(--line)] p-3 md:border-l md:border-t-0">
                <h2 className="mb-2 text-sm font-semibold">事件时间线</h2>
                <div className="grid max-h-full gap-2 overflow-auto">
                  {state.timeline.slice(0, 20).map((event, index) => (
                    <div key={`${event.type}-${index}`} className="rounded-md bg-[var(--panel-strong)] px-2 py-1 text-xs">
                      <span className="font-medium">{event.type}</span>
                      {event.kind || event.tool_name ? <span className="ml-2 text-[var(--muted)]">{event.kind ?? event.tool_name}</span> : null}
                      {event.content ? <span className="ml-2">{event.content}</span> : null}
                      {event.duration_ms ? <span className="ml-2 text-[var(--muted)]">{event.duration_ms}ms</span> : null}
                      {event.error ? <span className="ml-2 text-[var(--danger)]">{event.error}</span> : null}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
      <ApprovalDialog send={send} />
    </div>
  );
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function getDocumentStatus(doc: UploadedDocument) {
  if (doc.content.trim()) {
    return "已提取文本并加入上下文。";
  }
  if (doc.file_name.toLowerCase().endsWith(".pdf")) {
    return "当前 PDF 还未抽取到可读文本，可能需要 OCR；现在只能按已提取内容参与对话。";
  }
  return "已上传文档，但暂未提取到可读文本。";
}

function getRoleLabel(role: string) {
  if (role === "user") return "用户";
  if (role === "assistant") return "助手";
  if (role === "system") return "系统";
  return role;
}
