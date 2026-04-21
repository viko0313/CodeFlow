import type {
  AuditEvent,
  CodeFlowConfig,
  CodeFlowSession,
  Health,
  McpManifest,
  SkillManifest,
  UploadedDocument,
} from "@/lib/types";

const apiBase = "/api/codeflow";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: {
      "content-type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(payload?.error ?? `CodeFlow API returned ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export function getHealth() {
  return request<Health>("/health");
}

export function getConfig() {
  return request<CodeFlowConfig>("/config");
}

export async function getSessions() {
  const payload = await request<{ sessions: CodeFlowSession[] }>("/sessions");
  return payload.sessions;
}

export function createSession(title: string) {
  return request<CodeFlowSession>("/sessions", {
    method: "POST",
    body: JSON.stringify({ title }),
  });
}

export function switchSession(id: string) {
  return request<CodeFlowSession>(`/sessions/${id}/switch`, { method: "POST" });
}

export function deleteSession(id: string) {
  return request<{ deleted: string }>(`/sessions/${id}`, { method: "DELETE" });
}

export async function getRecentAudit() {
  const payload = await request<{ events: AuditEvent[] }>("/audit/recent?limit=20");
  return payload.events;
}

export function getSkills() {
  return request<SkillManifest>("/skills");
}

export function getMcp() {
  return request<McpManifest>("/mcp");
}

export async function uploadDocument(file: File) {
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${apiBase}/documents/upload`, {
    method: "POST",
    body: form,
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(payload?.error ?? `Upload failed with ${response.status}`);
  }
  return response.json() as Promise<UploadedDocument>;
}
