export type CodeFlowSession = {
  id: string;
  project_root: string;
  title: string;
  agent_md?: string;
  active: boolean;
  created_at: string;
  updated_at: string;
};

export type Health = {
  status: string;
  product: string;
  version: string;
  project_root: string;
  active_session_id: string;
  postgres_configured: boolean;
  redis_configured: boolean;
  model_configured: boolean;
  storage_backend: string;
  memory_backend: string;
  fallback_active: boolean;
};

export type CodeFlowConfig = {
  provider: string;
  model: string;
  base_url: string;
  api_key_configured: boolean;
  api_key_hint: string;
  project_root: string;
  data_dir: string;
  storage: {
    postgres_configured: boolean;
    redis_addr: string;
    redis_db: number;
  };
  runtime: {
    max_turns: number;
    max_actions: number;
  };
  agent: {
    mode: string;
    plan_enabled: boolean;
  };
  skills: {
    enabled: boolean;
    dirs: string[];
    preload: string[];
    max_content_bytes: number;
  };
  mcp: {
    enabled: boolean;
    preload: boolean;
    config_files: string[];
    servers: McpServer[];
  };
  documents: {
    upload_dir: string;
    max_upload_bytes: number;
    allowed_extensions: string[];
  };
};

export type AuditEvent = {
  time: string;
  session_id: string;
  project_root: string;
  operation_id?: string;
  event: string;
  tool_name?: string;
  args_summary?: string;
  result_summary?: string;
  duration_ms?: number;
  confirmed?: boolean;
};

export type ServerEvent = {
  type: string;
  id?: string;
  request_id?: string;
  session_id?: string;
  operation_id?: string;
  approval_id?: string;
  status?: string;
  kind?: string;
  path?: string;
  command?: string;
  preview?: string;
  risk?: string;
  timeout?: string;
  content?: string;
  output?: string;
  error?: string;
  reason?: string;
  duration_ms?: number;
  confirmed?: boolean;
};

export type ClientMessage = {
  type: string;
  id?: string;
  request_id?: string;
  session_id?: string;
  input?: string;
  command?: string;
  timeout_seconds?: number;
  path?: string;
  content?: string;
  append?: boolean;
  operation_id?: string;
  approval_id?: string;
  allowed?: boolean;
  reason?: string;
  plan_enabled?: boolean;
};

export type PendingApproval = {
  approval_id?: string;
  operation_id: string;
  request_id?: string;
  status?: string;
  kind: string;
  path?: string;
  command?: string;
  preview?: string;
  risk?: string;
  timeout?: string;
};

export type ApprovalRecord = {
  id: string;
  operation_id: string;
  session_id: string;
  project_root: string;
  kind: string;
  path?: string;
  command?: string;
  preview?: string;
  risk?: string;
  timeout?: string;
  request_id?: string;
  status: "pending" | "approved" | "rejected";
  decision_reason?: string;
  decided_at?: string;
  created_at: string;
  updated_at: string;
};

export type TaskEvent = {
  id: string;
  session_id?: string;
  request_id?: string;
  operation_id?: string;
  approval_id?: string;
  source: string;
  level: string;
  event_type: string;
  message: string;
  payload?: string;
  created_at: string;
};

export type Skill = {
  name: string;
  description: string;
  path: string;
  preloaded: boolean;
  content?: string;
};

export type SkillManifest = {
  enabled: boolean;
  dirs: string[];
  skills: Skill[];
};

export type McpServer = {
  name: string;
  command: string;
  args: string[];
  env?: Record<string, string>;
  enabled: boolean;
  source: string;
  preloaded: boolean;
};

export type McpManifest = {
  enabled: boolean;
  preload: boolean;
  config_files: string[];
  servers: McpServer[];
};

export type UploadedDocument = {
  id: string;
  file_name: string;
  path: string;
  size: number;
  chunks: number;
  content: string;
  created_at: string;
};

export type TraceEvent = {
  session_id: string;
  request_id: string;
  span_id?: string;
  parent_span_id?: string;
  iteration?: number;
  tool_name?: string;
  tool_call_id?: string;
  event_type: string;
  status?: string;
  duration_ms?: number;
  payload?: Record<string, unknown>;
  error_type?: string;
  created_at?: string;
};

export type EvalSummary = {
  session_id: string;
  requests: number;
  tool_calls: number;
  tool_failures: number;
  duplicates: number;
  total_duration_ms: number;
  final_status: string;
};

export type SessionHistoryTurn = {
  role: "user" | "assistant" | "system";
  content: string;
  created_at: string;
};

export type SessionHistory = {
  session_id: string;
  turns: SessionHistoryTurn[];
};
