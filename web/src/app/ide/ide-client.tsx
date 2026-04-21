"use client";

import dynamic from "next/dynamic";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileUp, Send, TerminalSquare, Wand2 } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { ApprovalDialog } from "@/components/ide/approval-dialog";
import { TerminalPanel } from "@/components/ide/terminal-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useCodeFlowSocket } from "@/hooks/use-codeflow-socket";
import { getConfig, getSessions, switchSession, uploadDocument } from "@/lib/codeflow-api";
import type { UploadedDocument } from "@/lib/types";
import { useUiStore } from "@/stores/use-ui-store";

const MonacoEditor = dynamic(() => import("@monaco-editor/react"), { ssr: false });

export function IdeClient() {
  const params = useSearchParams();
  const queryClient = useQueryClient();
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: getSessions });
  const config = useQuery({ queryKey: ["config"], queryFn: getConfig });
  const activeFromStore = useUiStore((store) => store.activeSessionId);
  const setActiveSessionId = useUiStore((store) => store.setActiveSessionId);
  const socketStatus = useUiStore((store) => store.socketStatus);
  const planEnabled = useUiStore((store) => store.planEnabled);
  const setPlanEnabled = useUiStore((store) => store.setPlanEnabled);
  const editorPath = useUiStore((store) => store.editorPath);
  const editorBuffer = useUiStore((store) => store.editorBuffer);
  const setEditor = useUiStore((store) => store.setEditor);
  const initialSession = params.get("session") ?? sessions.data?.find((session) => session.active)?.id ?? sessions.data?.[0]?.id;
  const activeSessionId = activeFromStore ?? initialSession;
  const { state, send, sendChat } = useCodeFlowSocket(activeSessionId);
  const [prompt, setPrompt] = useState("");
  const [command, setCommand] = useState("git status --short");
  const [uploadedDocs, setUploadedDocs] = useState<UploadedDocument[]>([]);

  useEffect(() => {
    if (initialSession && !activeFromStore) {
      setActiveSessionId(initialSession);
    }
  }, [activeFromStore, initialSession, setActiveSessionId]);

  useEffect(() => {
    if (config.data?.agent) {
      setPlanEnabled(config.data.agent.plan_enabled);
    }
  }, [config.data?.agent, setPlanEnabled]);

  const switchMutation = useMutation({
    mutationFn: switchSession,
    onSuccess: (session) => {
      setActiveSessionId(session.id);
      send({ type: "session.switch", session_id: session.id });
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });

  const activeTitle = useMemo(
    () => sessions.data?.find((session) => session.id === activeSessionId)?.title ?? "Workspace",
    [activeSessionId, sessions.data],
  );

  function submitPrompt(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = prompt.trim();
    if (!value) return;
    sendChat(value);
    setPrompt("");
  }

  function runCommand(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!command.trim()) return;
    send({ type: "terminal.run", id: crypto.randomUUID(), command });
  }

  function previewFile() {
    send({
      type: "file.preview",
      id: crypto.randomUUID(),
      path: editorPath,
      content: editorBuffer,
    });
  }

  function writeFile() {
    send({
      type: "file.write",
      id: crypto.randomUUID(),
      path: editorPath,
      content: editorBuffer,
    });
  }

  async function uploadSelectedFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) return;
    const doc = await uploadDocument(file);
    setUploadedDocs((items) => [doc, ...items].slice(0, 5));
    event.target.value = "";
  }

  return (
    <div className="grid h-[calc(100vh-66px)] min-h-[760px] grid-cols-1 overflow-hidden lg:grid-cols-[260px_1fr]">
      <aside className="border-b border-[var(--line)] bg-white p-4 lg:border-b-0 lg:border-r">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-semibold">Sessions</h2>
          <Badge className={socketStatus === "online" ? "border-[var(--accent)] text-[var(--accent-strong)]" : ""}>
            {socketStatus}
          </Badge>
        </div>
        <div className="grid max-h-72 gap-2 overflow-auto lg:max-h-[calc(100vh-160px)]">
          {(sessions.data ?? []).map((session) => (
            <button
              key={session.id}
              className={`rounded-lg border px-3 py-2 text-left text-sm ${
                session.id === activeSessionId
                  ? "border-[var(--accent)] bg-[var(--panel-strong)]"
                  : "border-[var(--line)] bg-white hover:bg-[var(--panel-strong)]"
              }`}
              onClick={() => switchMutation.mutate(session.id)}
            >
              <span className="block truncate font-medium">{session.title}</span>
              <span className="block truncate text-xs text-[var(--muted)]">{session.id}</span>
            </button>
          ))}
        </div>
      </aside>

      <section className="grid min-h-0 grid-rows-[auto_1fr]">
        <div className="flex items-center justify-between gap-3 border-b border-[var(--line)] bg-white px-4 py-3">
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold">{activeTitle}</h1>
            <p className="truncate text-xs text-[var(--muted)]">
              {activeSessionId ?? "No active session"} · agent {config.data?.agent?.mode ?? "react"}
            </p>
          </div>
          <div className="flex gap-2">
            <label className="inline-flex h-8 cursor-pointer items-center gap-2 rounded-lg border border-[var(--line)] bg-white px-3 text-xs font-medium hover:bg-[var(--panel-strong)]">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[var(--accent)]"
                checked={planEnabled}
                onChange={(event) => setPlanEnabled(event.target.checked)}
              />
              Plan
            </label>
            <label className="inline-flex h-8 cursor-pointer items-center gap-2 rounded-lg border border-[var(--line)] bg-white px-3 text-xs font-medium hover:bg-[var(--panel-strong)]">
              <FileUp className="h-4 w-4" />
              Upload doc
              <input className="hidden" type="file" onChange={uploadSelectedFile} />
            </label>
            <Button variant="secondary" size="sm" onClick={previewFile}>
              Preview diff
            </Button>
            <Button size="sm" onClick={writeFile}>
              Write file
            </Button>
          </div>
        </div>

        <div className="grid min-h-0 gap-0 xl:grid-cols-[0.95fr_1.05fr]">
          <div className="grid min-h-0 grid-rows-[1fr_300px] border-r border-[var(--line)]">
            <div className="grid min-h-0 grid-rows-[1fr_auto] bg-white">
              <div className="overflow-auto p-4">
                <div className="grid gap-3">
                  {state.chat.map((message) => (
                    <div
                      key={`${message.role}-${message.id}-${message.content.length}`}
                      className={`rounded-lg border border-[var(--line)] p-3 text-sm leading-6 ${
                        message.role === "user" ? "bg-[var(--panel-strong)]" : "bg-white"
                      }`}
                    >
                      <p className="mb-1 text-xs font-semibold uppercase text-[var(--muted)]">{message.role}</p>
                      <p className="whitespace-pre-wrap">{message.content}</p>
                    </div>
                  ))}
                  {!state.chat.length ? (
                    <p className="rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
                      Ask CodeFlow to inspect, explain, or plan changes for this project.
                    </p>
                  ) : null}
                </div>
              </div>
              <form className="border-t border-[var(--line)] p-3" onSubmit={submitPrompt}>
                {uploadedDocs.length ? (
                  <div className="mb-2 flex flex-wrap gap-2">
                    {uploadedDocs.map((doc) => (
                      <Badge key={doc.id}>
                        {doc.file_name} · {doc.chunks} chunk{doc.chunks === 1 ? "" : "s"}
                      </Badge>
                    ))}
                  </div>
                ) : null}
                <div className="flex gap-2">
                  <Input
                    value={prompt}
                    onChange={(event) => setPrompt(event.target.value)}
                    placeholder="Ask CodeFlow"
                  />
                  <Button type="submit">
                    <Send className="h-4 w-4" />
                    Send
                  </Button>
                </div>
              </form>
            </div>

            <div className="grid min-h-0 grid-rows-[auto_1fr] border-t border-[var(--line)] bg-[var(--terminal)]">
              <form className="flex gap-2 border-b border-white/10 p-3" onSubmit={runCommand}>
                <Input
                  className="border-white/20 bg-[#24282c] text-white"
                  value={command}
                  onChange={(event) => setCommand(event.target.value)}
                />
                <Button type="submit" variant="secondary">
                  <TerminalSquare className="h-4 w-4" />
                  Run
                </Button>
              </form>
              <TerminalPanel output={state.terminal.join("\r\n")} />
            </div>
          </div>

          <div className="grid min-h-0 grid-rows-[auto_1fr_220px] bg-white">
            <div className="flex flex-wrap items-center gap-2 border-b border-[var(--line)] p-3">
              <Input
                className="max-w-sm"
                value={editorPath}
                onChange={(event) => setEditor(event.target.value, editorBuffer)}
              />
              <Badge>{state.latestDiff?.path ?? "No diff yet"}</Badge>
            </div>
            <MonacoEditor
              height="100%"
              language="markdown"
              path={editorPath}
              value={editorBuffer}
              theme="vs-light"
              options={{ minimap: { enabled: false }, fontSize: 14, wordWrap: "on" }}
              onChange={(value) => setEditor(editorPath, value ?? "")}
            />
            <div className="grid min-h-0 border-t border-[var(--line)] md:grid-cols-[1fr_280px]">
              <div className="min-h-0 p-3">
                <div className="mb-2 flex items-center gap-2">
                  <Wand2 className="h-4 w-4 text-[var(--accent-strong)]" />
                  <h2 className="text-sm font-semibold">Diff Preview</h2>
                </div>
                <Textarea
                  readOnly
                  className="h-40 font-mono text-xs"
                  value={state.latestDiff?.preview ?? "Preview or write a file to see the diff."}
                />
              </div>
              <div className="min-h-0 border-t border-[var(--line)] p-3 md:border-l md:border-t-0">
                <h2 className="mb-2 text-sm font-semibold">Event Timeline</h2>
                <div className="grid max-h-44 gap-2 overflow-auto">
                  {state.timeline.slice(0, 20).map((event, index) => (
                    <div key={`${event.type}-${index}`} className="rounded-md bg-[var(--panel-strong)] px-2 py-1 text-xs">
                      <span className="font-medium">{event.type}</span>
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
