package storage

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
	"github.com/viko0313/CodeFlow/internal/codeflow/workspace"
)

type PostgresSessionStore struct {
	ctx  context.Context
	pool *pgxpool.Pool
}

func NewPostgresSessionStore(ctx context.Context, dsn string) (*PostgresSessionStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL is required for CodeFlow sessions; set CODEFLOW_POSTGRES_DSN")
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(connectCtx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	store := &PostgresSessionStore{ctx: ctx, pool: pool}
	if err := store.migrate(); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresSessionStore) migrate() error {
	_, err := s.pool.Exec(s.ctx, `
CREATE TABLE IF NOT EXISTS codeflow_sessions (
  id TEXT PRIMARY KEY,
  project_root TEXT NOT NULL,
  title TEXT NOT NULL,
  agent_md TEXT NOT NULL DEFAULT '',
  active BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_sessions_project_root ON codeflow_sessions(project_root);
CREATE TABLE IF NOT EXISTS codeflow_approvals (
  id TEXT PRIMARY KEY,
  operation_id TEXT NOT NULL UNIQUE,
  session_id TEXT NOT NULL DEFAULT '',
  project_root TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  path TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL DEFAULT '',
  preview TEXT NOT NULL DEFAULT '',
  risk TEXT NOT NULL DEFAULT '',
  timeout TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  decision_reason TEXT NOT NULL DEFAULT '',
  decided_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_approvals_status_created ON codeflow_approvals(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codeflow_approvals_session_created ON codeflow_approvals(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS task_events (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  operation_id TEXT NOT NULL DEFAULT '',
  approval_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_task_events_created ON task_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_events_session_created ON task_events(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  request_id TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_messages_session_created ON codeflow_messages(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_model_configs (
  project_root TEXT PRIMARY KEY,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  api_key_ciphertext TEXT NOT NULL DEFAULT '',
  api_key_hint TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS codeflow_workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL UNIQUE,
  config_path TEXT NOT NULL DEFAULT '',
  agent_md_path TEXT NOT NULL DEFAULT '',
  default_branch TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  active BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  last_opened_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_workspaces_active ON codeflow_workspaces(active, last_opened_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  plan_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  model_provider TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL DEFAULT '',
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_codeflow_runs_session_started ON codeflow_runs(session_id, started_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_run_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  type TEXT NOT NULL,
  timestamp TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  latency_ms BIGINT NOT NULL DEFAULT 0,
  request_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_codeflow_run_events_run_time ON codeflow_run_events(run_id, timestamp ASC);
CREATE TABLE IF NOT EXISTS codeflow_plans (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  goal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  preference JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS codeflow_plan_steps (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  requires_approval BOOLEAN NOT NULL DEFAULT false,
  related_files JSONB NOT NULL DEFAULT '[]'::jsonb,
  tool_calls JSONB NOT NULL DEFAULT '[]'::jsonb,
  result_summary TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_codeflow_plan_steps_plan_position ON codeflow_plan_steps(plan_id, position ASC);
CREATE TABLE IF NOT EXISTS codeflow_plan_events (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL,
  step_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_plan_events_plan_time ON codeflow_plan_events(plan_id, created_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_checkpoints (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  plan_step_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  git_head TEXT NOT NULL DEFAULT '',
  changed_files JSONB NOT NULL DEFAULT '[]'::jsonb,
  snapshot_path TEXT NOT NULL DEFAULT '',
  patch_path TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS codeflow_checkpoint_files (
  id TEXT PRIMARY KEY,
  checkpoint_id TEXT NOT NULL,
  path TEXT NOT NULL,
  existed BOOLEAN NOT NULL DEFAULT false,
  is_binary BOOLEAN NOT NULL DEFAULT false,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  sha256 TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_checkpoint_files_checkpoint ON codeflow_checkpoint_files(checkpoint_id);
CREATE TABLE IF NOT EXISTS codeflow_session_summaries (
  session_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL
);
`)
	if err != nil {
		return fmt.Errorf("migrate CodeFlow session schema: %w", err)
	}
	return nil
}

func (s *PostgresSessionStore) GetModelConfig(projectRoot string) (*ModelConfig, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT project_root, provider, model, base_url, api_key_ciphertext, api_key_hint, created_at, updated_at
FROM codeflow_model_configs
WHERE project_root=$1
`, projectRoot)
	item, err := scanModelConfig(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *PostgresSessionStore) UpsertModelConfig(projectRoot string, input UpsertModelConfigInput) (*ModelConfig, error) {
	now := time.Now().UTC()
	apiKeyCiphertext := ""
	apiKeyHint := ""
	if input.APIKeyCiphertext != nil {
		apiKeyCiphertext = *input.APIKeyCiphertext
	}
	if input.APIKeyHint != nil {
		apiKeyHint = *input.APIKeyHint
	}
	if input.APIKeyCiphertext == nil && input.APIKeyHint == nil {
		row := s.pool.QueryRow(s.ctx, `
INSERT INTO codeflow_model_configs (project_root, provider, model, base_url, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$5)
ON CONFLICT (project_root) DO UPDATE SET
  provider=EXCLUDED.provider,
  model=EXCLUDED.model,
  base_url=EXCLUDED.base_url,
  updated_at=EXCLUDED.updated_at
RETURNING project_root, provider, model, base_url, api_key_ciphertext, api_key_hint, created_at, updated_at
`, projectRoot, input.Provider, input.Model, input.BaseURL, now)
		return scanModelConfig(row)
	}
	row := s.pool.QueryRow(s.ctx, `
INSERT INTO codeflow_model_configs (project_root, provider, model, base_url, api_key_ciphertext, api_key_hint, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$7)
ON CONFLICT (project_root) DO UPDATE SET
  provider=EXCLUDED.provider,
  model=EXCLUDED.model,
  base_url=EXCLUDED.base_url,
  api_key_ciphertext=EXCLUDED.api_key_ciphertext,
  api_key_hint=EXCLUDED.api_key_hint,
  updated_at=EXCLUDED.updated_at
RETURNING project_root, provider, model, base_url, api_key_ciphertext, api_key_hint, created_at, updated_at
`, projectRoot, input.Provider, input.Model, input.BaseURL, apiKeyCiphertext, apiKeyHint, now)
	return scanModelConfig(row)
}

func (s *PostgresSessionStore) Create(projectRoot, title, agentMD string) (*cfsession.Session, error) {
	now := time.Now().UTC()
	id := newSessionID(projectRoot)
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(s.ctx)
	if _, err := tx.Exec(s.ctx, `UPDATE codeflow_sessions SET active=false WHERE project_root=$1`, projectRoot); err != nil {
		return nil, err
	}
	_, err = tx.Exec(s.ctx, `
INSERT INTO codeflow_sessions (id, project_root, title, agent_md, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, true, $5, $5)
`, id, projectRoot, defaultTitle(title), agentMD, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(s.ctx); err != nil {
		return nil, err
	}
	return &cfsession.Session{ID: id, ProjectRoot: projectRoot, Title: defaultTitle(title), AgentMD: agentMD, Active: true, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *PostgresSessionStore) GetActive(projectRoot string) (*cfsession.Session, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT id, project_root, title, agent_md, active, created_at, updated_at
FROM codeflow_sessions
WHERE project_root=$1 AND active=true
ORDER BY updated_at DESC
LIMIT 1
`, projectRoot)
	session, err := scanSession(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return session, err
}

func (s *PostgresSessionStore) List(projectRoot string) ([]cfsession.Session, error) {
	rows, err := s.pool.Query(s.ctx, `
SELECT id, project_root, title, agent_md, active, created_at, updated_at
FROM codeflow_sessions
WHERE project_root=$1
ORDER BY updated_at DESC
`, projectRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []cfsession.Session
	for rows.Next() {
		var item cfsession.Session
		if err := rows.Scan(&item.ID, &item.ProjectRoot, &item.Title, &item.AgentMD, &item.Active, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) Switch(projectRoot, sessionID string) (*cfsession.Session, error) {
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(s.ctx)
	tag, err := tx.Exec(s.ctx, `UPDATE codeflow_sessions SET active=false WHERE project_root=$1`, projectRoot)
	_ = tag
	if err != nil {
		return nil, err
	}
	row := tx.QueryRow(s.ctx, `
UPDATE codeflow_sessions
SET active=true, updated_at=$3
WHERE project_root=$1 AND id=$2
RETURNING id, project_root, title, agent_md, active, created_at, updated_at
`, projectRoot, sessionID, time.Now().UTC())
	session, err := scanSession(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(s.ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *PostgresSessionStore) Delete(projectRoot, sessionID string) error {
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(s.ctx)
	if _, err := tx.Exec(s.ctx, `DELETE FROM codeflow_messages WHERE session_id=$1`, sessionID); err != nil {
		return err
	}
	tag, err := tx.Exec(s.ctx, `DELETE FROM codeflow_sessions WHERE project_root=$1 AND id=$2`, projectRoot, sessionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return tx.Commit(s.ctx)
}

func (s *PostgresSessionStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresSessionStore) CreateApproval(input CreateApprovalInput) (*ApprovalRecord, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "apr_" + uuid.NewString()[:8]
	}
	row := s.pool.QueryRow(s.ctx, `
INSERT INTO codeflow_approvals (
  id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'pending','',NULL,$12,$12)
RETURNING id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
`, id, input.OperationID, input.SessionID, input.ProjectRoot, input.Kind, input.Path, input.Command, input.Preview, input.Risk, input.Timeout, input.RequestID, now)
	record, err := scanApproval(row)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *PostgresSessionStore) GetApproval(id string) (*ApprovalRecord, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
WHERE id=$1
`, id)
	record, err := scanApproval(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *PostgresSessionStore) GetApprovalByOperationID(operationID string) (*ApprovalRecord, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
WHERE operation_id=$1
`, operationID)
	record, err := scanApproval(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *PostgresSessionStore) ListApprovals(opts ListApprovalsOptions) ([]ApprovalRecord, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	status := strings.TrimSpace(opts.Status)
	var (
		rows pgx.Rows
		err  error
	)
	if status == "" {
		rows, err = s.pool.Query(s.ctx, `
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
ORDER BY created_at DESC
LIMIT $1
`, limit)
	} else {
		rows, err = s.pool.Query(s.ctx, `
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
WHERE status=$1
ORDER BY created_at DESC
LIMIT $2
`, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ApprovalRecord, 0, limit)
	for rows.Next() {
		record, scanErr := scanApproval(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) Register(rootPath, name string, metadata map[string]string) (*workspace.Workspace, error) {
	now := time.Now().UTC()
	id := "ws_" + uuid.NewString()[:8]
	if existing, err := s.GetWorkspaceByRoot(rootPath); err != nil {
		return nil, err
	} else if existing != nil {
		return s.SwitchWorkspace(existing.ID)
	}
	data, _ := json.Marshal(metadata)
	configPath := metadata["config_path"]
	agentMDPath := metadata["agent_md_path"]
	defaultBranch := metadata["default_branch"]
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(s.ctx)
	if _, err := tx.Exec(s.ctx, `UPDATE codeflow_workspaces SET active=false`); err != nil {
		return nil, err
	}
	_, err = tx.Exec(s.ctx, `
INSERT INTO codeflow_workspaces (id, name, root_path, config_path, agent_md_path, default_branch, metadata, active, created_at, updated_at, last_opened_at)
VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,true,$8,$8,$8)
`, id, defaultTitle(name), rootPath, configPath, agentMDPath, defaultBranch, string(data), now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(s.ctx); err != nil {
		return nil, err
	}
	return s.GetWorkspaceByID(id)
}

func (s *PostgresSessionStore) ListWorkspaces() ([]workspace.Workspace, error) {
	rows, err := s.pool.Query(s.ctx, `
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata::text, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
ORDER BY active DESC, last_opened_at DESC, created_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []workspace.Workspace{}
	for rows.Next() {
		item, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) GetWorkspaceByID(id string) (*workspace.Workspace, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata::text, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
WHERE id=$1
`, id)
	item, err := scanWorkspace(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *PostgresSessionStore) GetWorkspaceByRoot(rootPath string) (*workspace.Workspace, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata::text, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
WHERE root_path=$1
`, rootPath)
	item, err := scanWorkspace(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *PostgresSessionStore) GetCurrentWorkspace() (*workspace.Workspace, error) {
	row := s.pool.QueryRow(s.ctx, `
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata::text, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
WHERE active=true
ORDER BY last_opened_at DESC
LIMIT 1
`)
	item, err := scanWorkspace(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *PostgresSessionStore) SwitchWorkspace(id string) (*workspace.Workspace, error) {
	now := time.Now().UTC()
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(s.ctx)
	if _, err := tx.Exec(s.ctx, `UPDATE codeflow_workspaces SET active=false`); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(s.ctx, `UPDATE codeflow_workspaces SET active=true, updated_at=$2, last_opened_at=$2 WHERE id=$1`, id, now)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	if err := tx.Commit(s.ctx); err != nil {
		return nil, err
	}
	return s.GetWorkspaceByID(id)
}

func (s *PostgresSessionStore) RemoveWorkspace(id string) error {
	_, err := s.pool.Exec(s.ctx, `DELETE FROM codeflow_workspaces WHERE id=$1`, id)
	return err
}

func (s *PostgresSessionStore) DecideApproval(id string, allowed bool, reason string) (*ApprovalRecord, error) {
	reason = strings.TrimSpace(reason)
	status := ApprovalStatusRejected
	if allowed {
		status = ApprovalStatusApproved
	}
	decidedAt := time.Now().UTC()
	row := s.pool.QueryRow(s.ctx, `
UPDATE codeflow_approvals
SET status=$2, decision_reason=$3, decided_at=$4, updated_at=$4
WHERE id=$1 AND status='pending'
RETURNING id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
`, id, status, reason, decidedAt)
	record, err := scanApproval(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *PostgresSessionStore) CreateTaskEvent(input CreateTaskEventInput) (*TaskEvent, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "evt_" + uuid.NewString()[:8]
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	payload := strings.TrimSpace(input.Payload)
	if payload == "" {
		payload = "{}"
	}
	row := s.pool.QueryRow(s.ctx, `
INSERT INTO task_events (id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)
RETURNING id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload::text, created_at
`, id, input.SessionID, input.RequestID, input.OperationID, input.ApprovalID, input.Source, input.Level, input.EventType, input.Message, payload, createdAt)
	event, err := scanTaskEvent(row)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (s *PostgresSessionStore) ListTaskEvents(opts ListTaskEventsOptions) ([]TaskEvent, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	sessionID := strings.TrimSpace(opts.SessionID)
	var (
		rows pgx.Rows
		err  error
	)
	if sessionID == "" {
		rows, err = s.pool.Query(s.ctx, `
SELECT id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload::text, created_at
FROM task_events
ORDER BY created_at DESC, id DESC
LIMIT $1
`, limit)
	} else {
		rows, err = s.pool.Query(s.ctx, `
SELECT id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload::text, created_at
FROM task_events
WHERE session_id=$1
ORDER BY created_at DESC, id DESC
LIMIT $2
`, sessionID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TaskEvent, 0, limit)
	for rows.Next() {
		item, scanErr := scanTaskEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) AppendMessage(ctx context.Context, input MessageRecord) error {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "msg_" + uuid.NewString()[:8]
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO codeflow_messages (id, session_id, request_id, role, content, tool_call_id, tool_name, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
`, id, input.SessionID, input.RequestID, input.Role, input.Content, input.ToolCallID, input.ToolName, createdAt)
	return err
}

func (s *PostgresSessionStore) ListMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, session_id, request_id, role, content, tool_call_id, tool_name, created_at
FROM codeflow_messages
WHERE session_id=$1
ORDER BY created_at DESC, id DESC
LIMIT $2
`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MessageRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *item)
	}
	reverseMessages(out)
	return out, rows.Err()
}

func (s *PostgresSessionStore) SearchMessages(ctx context.Context, sessionID, query string, limit int) ([]MessageSearchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []MessageSearchResult{}, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, session_id, request_id, role, content, tool_call_id, tool_name, created_at
FROM codeflow_messages
WHERE session_id=$1 AND content ILIKE $2
ORDER BY created_at DESC, id DESC
LIMIT $3
`, sessionID, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MessageSearchResult, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, MessageSearchResult{MessageRecord: *item, Snippet: snippet(item.Content, query)})
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (*cfsession.Session, error) {
	var item cfsession.Session
	if err := row.Scan(&item.ID, &item.ProjectRoot, &item.Title, &item.AgentMD, &item.Active, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanApproval(row scanner) (*ApprovalRecord, error) {
	var item ApprovalRecord
	if err := row.Scan(
		&item.ID,
		&item.OperationID,
		&item.SessionID,
		&item.ProjectRoot,
		&item.Kind,
		&item.Path,
		&item.Command,
		&item.Preview,
		&item.Risk,
		&item.Timeout,
		&item.RequestID,
		&item.Status,
		&item.DecisionReason,
		&item.DecidedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanTaskEvent(row scanner) (*TaskEvent, error) {
	var item TaskEvent
	if err := row.Scan(
		&item.ID,
		&item.SessionID,
		&item.RequestID,
		&item.OperationID,
		&item.ApprovalID,
		&item.Source,
		&item.Level,
		&item.EventType,
		&item.Message,
		&item.Payload,
		&item.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanMessage(row scanner) (*MessageRecord, error) {
	var item MessageRecord
	if err := row.Scan(
		&item.ID,
		&item.SessionID,
		&item.RequestID,
		&item.Role,
		&item.Content,
		&item.ToolCallID,
		&item.ToolName,
		&item.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanModelConfig(row scanner) (*ModelConfig, error) {
	var item ModelConfig
	if err := row.Scan(
		&item.ProjectRoot,
		&item.Provider,
		&item.Model,
		&item.BaseURL,
		&item.APIKeyCiphertext,
		&item.APIKeyHint,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanWorkspace(row scanner) (*workspace.Workspace, error) {
	var (
		item         workspace.Workspace
		metadataJSON string
	)
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.RootPath,
		&item.ConfigPath,
		&item.AgentMDPath,
		&item.DefaultBranch,
		&metadataJSON,
		&item.Active,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastOpenedAt,
	); err != nil {
		return nil, err
	}
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &item.Metadata)
	}
	return &item, nil
}

func reverseMessages(items []MessageRecord) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func snippet(content, query string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 180 {
		return content
	}
	idx := strings.Index(strings.ToLower(content), strings.ToLower(query))
	if idx < 0 {
		return content[:180]
	}
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := start + 180
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}

func newSessionID(projectRoot string) string {
	sum := sha1.Sum([]byte(projectRoot))
	return "cf_" + hex.EncodeToString(sum[:])[:8] + "_" + uuid.NewString()[:8]
}

func defaultTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "CodeFlow session"
	}
	return strings.TrimSpace(title)
}
