package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
	"github.com/viko0313/CodeFlow/internal/codeflow/workspace"
)

type SQLiteSessionStore struct {
	db *sql.DB
}

func NewSQLiteSessionStore(dbPath string) (*SQLiteSessionStore, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return nil, fmt.Errorf("sqlite db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &SQLiteSessionStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteSessionStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS codeflow_sessions (
  id TEXT PRIMARY KEY,
  project_root TEXT NOT NULL,
  title TEXT NOT NULL,
  agent_md TEXT NOT NULL DEFAULT '',
  active INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
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
  decided_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
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
  payload TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
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
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_messages_session_created ON codeflow_messages(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_model_configs (
  project_root TEXT PRIMARY KEY,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  api_key_ciphertext TEXT NOT NULL DEFAULT '',
  api_key_hint TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS codeflow_workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL UNIQUE,
  config_path TEXT NOT NULL DEFAULT '',
  agent_md_path TEXT NOT NULL DEFAULT '',
  default_branch TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  active INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_opened_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_workspaces_active ON codeflow_workspaces(active, last_opened_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  plan_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
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
  timestamp TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT '{}',
  latency_ms INTEGER NOT NULL DEFAULT 0,
  request_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_codeflow_run_events_run_time ON codeflow_run_events(run_id, timestamp ASC);
CREATE TABLE IF NOT EXISTS codeflow_plans (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  goal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  preference TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS codeflow_plan_steps (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  requires_approval INTEGER NOT NULL DEFAULT 0,
  related_files TEXT NOT NULL DEFAULT '[]',
  tool_calls TEXT NOT NULL DEFAULT '[]',
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
  payload TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_plan_events_plan_time ON codeflow_plan_events(plan_id, created_at DESC);
CREATE TABLE IF NOT EXISTS codeflow_checkpoints (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  plan_step_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  git_head TEXT NOT NULL DEFAULT '',
  changed_files TEXT NOT NULL DEFAULT '[]',
  snapshot_path TEXT NOT NULL DEFAULT '',
  patch_path TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS codeflow_checkpoint_files (
  id TEXT PRIMARY KEY,
  checkpoint_id TEXT NOT NULL,
  path TEXT NOT NULL,
  existed INTEGER NOT NULL DEFAULT 0,
  is_binary INTEGER NOT NULL DEFAULT 0,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codeflow_checkpoint_files_checkpoint ON codeflow_checkpoint_files(checkpoint_id);
CREATE TABLE IF NOT EXISTS codeflow_session_summaries (
  session_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
`)
	return err
}

func (s *SQLiteSessionStore) Create(projectRoot, title, agentMD string) (*cfsession.Session, error) {
	now := time.Now().UTC()
	id := newSessionID(projectRoot)
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE codeflow_sessions SET active=0 WHERE project_root=?`, projectRoot); err != nil {
		return nil, err
	}
	_, err = tx.Exec(`
INSERT INTO codeflow_sessions (id, project_root, title, agent_md, active, created_at, updated_at)
VALUES (?, ?, ?, ?, 1, ?, ?)
`, id, projectRoot, defaultTitle(title), agentMD, formatTS(now), formatTS(now))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &cfsession.Session{
		ID:          id,
		ProjectRoot: projectRoot,
		Title:       defaultTitle(title),
		AgentMD:     agentMD,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *SQLiteSessionStore) GetActive(projectRoot string) (*cfsession.Session, error) {
	row := s.db.QueryRow(`
SELECT id, project_root, title, agent_md, active, created_at, updated_at
FROM codeflow_sessions
WHERE project_root=? AND active=1
ORDER BY updated_at DESC
LIMIT 1
`, projectRoot)
	item, err := scanSQLiteSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) List(projectRoot string) ([]cfsession.Session, error) {
	rows, err := s.db.Query(`
SELECT id, project_root, title, agent_md, active, created_at, updated_at
FROM codeflow_sessions
WHERE project_root=?
ORDER BY updated_at DESC
`, projectRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []cfsession.Session
	for rows.Next() {
		item, scanErr := scanSQLiteSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) Switch(projectRoot, sessionID string) (*cfsession.Session, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE codeflow_sessions SET active=0 WHERE project_root=?`, projectRoot); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE codeflow_sessions SET active=1, updated_at=? WHERE project_root=? AND id=?`, formatTS(now), projectRoot, sessionID); err != nil {
		return nil, err
	}
	row := tx.QueryRow(`
SELECT id, project_root, title, agent_md, active, created_at, updated_at
FROM codeflow_sessions
WHERE project_root=? AND id=?
`, projectRoot, sessionID)
	item, err := scanSQLiteSession(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *SQLiteSessionStore) Delete(projectRoot, sessionID string) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM codeflow_messages WHERE session_id=?`, sessionID); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM codeflow_sessions WHERE project_root=? AND id=?`, projectRoot, sessionID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return tx.Commit()
}

func (s *SQLiteSessionStore) CreateApproval(input CreateApprovalInput) (*ApprovalRecord, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "apr_" + uuid.NewString()[:8]
	}
	_, err := s.db.Exec(`
INSERT INTO codeflow_approvals (
  id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', NULL, ?, ?)
`, id, input.OperationID, input.SessionID, input.ProjectRoot, input.Kind, input.Path, input.Command, input.Preview, input.Risk, input.Timeout, input.RequestID, formatTS(now), formatTS(now))
	if err != nil {
		return nil, err
	}
	return s.GetApproval(id)
}

func (s *SQLiteSessionStore) GetApproval(id string) (*ApprovalRecord, error) {
	row := s.db.QueryRow(`
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
WHERE id=?
`, id)
	item, err := scanSQLiteApproval(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) GetApprovalByOperationID(operationID string) (*ApprovalRecord, error) {
	row := s.db.QueryRow(`
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
WHERE operation_id=?
`, operationID)
	item, err := scanSQLiteApproval(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) ListApprovals(opts ListApprovalsOptions) ([]ApprovalRecord, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	status := strings.TrimSpace(opts.Status)
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		rows, err = s.db.Query(`
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
ORDER BY created_at DESC
LIMIT ?
`, limit)
	} else {
		rows, err = s.db.Query(`
SELECT id, operation_id, session_id, project_root, kind, path, command, preview, risk, timeout, request_id, status, decision_reason, decided_at, created_at, updated_at
FROM codeflow_approvals
WHERE status=?
ORDER BY created_at DESC
LIMIT ?
`, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ApprovalRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanSQLiteApproval(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) DecideApproval(id string, allowed bool, reason string) (*ApprovalRecord, error) {
	reason = strings.TrimSpace(reason)
	status := ApprovalStatusRejected
	if allowed {
		status = ApprovalStatusApproved
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
UPDATE codeflow_approvals
SET status=?, decision_reason=?, decided_at=?, updated_at=?
WHERE id=? AND status='pending'
`, string(status), reason, formatTS(now), formatTS(now), id)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, nil
	}
	return s.GetApproval(id)
}

func (s *SQLiteSessionStore) CreateTaskEvent(input CreateTaskEventInput) (*TaskEvent, error) {
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
	_, err := s.db.Exec(`
INSERT INTO task_events (id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.SessionID, input.RequestID, input.OperationID, input.ApprovalID, input.Source, input.Level, input.EventType, input.Message, payload, formatTS(createdAt))
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`
SELECT id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload, created_at
FROM task_events
WHERE id=?
`, id)
	return scanSQLiteTaskEvent(row)
}

func (s *SQLiteSessionStore) ListTaskEvents(opts ListTaskEventsOptions) ([]TaskEvent, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	sessionID := strings.TrimSpace(opts.SessionID)
	var (
		rows *sql.Rows
		err  error
	)
	if sessionID == "" {
		rows, err = s.db.Query(`
SELECT id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload, created_at
FROM task_events
ORDER BY created_at DESC, id DESC
LIMIT ?
`, limit)
	} else {
		rows, err = s.db.Query(`
SELECT id, session_id, request_id, operation_id, approval_id, source, level, event_type, message, payload, created_at
FROM task_events
WHERE session_id=?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, sessionID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TaskEvent, 0, limit)
	for rows.Next() {
		item, scanErr := scanSQLiteTaskEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) AppendMessage(ctx context.Context, input MessageRecord) error {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "msg_" + uuid.NewString()[:8]
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO codeflow_messages (id, session_id, request_id, role, content, tool_call_id, tool_name, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.SessionID, input.RequestID, input.Role, input.Content, input.ToolCallID, input.ToolName, formatTS(createdAt))
	return err
}

func (s *SQLiteSessionStore) ListMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, request_id, role, content, tool_call_id, tool_name, created_at
FROM codeflow_messages
WHERE session_id=?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MessageRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanSQLiteMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *item)
	}
	reverseMessages(out)
	return out, rows.Err()
}

func (s *SQLiteSessionStore) SearchMessages(ctx context.Context, sessionID, query string, limit int) ([]MessageSearchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []MessageSearchResult{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, request_id, role, content, tool_call_id, tool_name, created_at
FROM codeflow_messages
WHERE session_id=? AND lower(content) LIKE lower(?)
ORDER BY created_at DESC, id DESC
LIMIT ?
`, sessionID, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MessageSearchResult, 0, limit)
	for rows.Next() {
		item, scanErr := scanSQLiteMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, MessageSearchResult{MessageRecord: *item, Snippet: snippet(item.Content, query)})
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) Close() {
	if s.db != nil {
		_ = s.db.Close()
	}
}

func (s *SQLiteSessionStore) GetModelConfig(projectRoot string) (*ModelConfig, error) {
	row := s.db.QueryRow(`
SELECT project_root, provider, model, base_url, api_key_ciphertext, api_key_hint, created_at, updated_at
FROM codeflow_model_configs
WHERE project_root=?
`, projectRoot)
	item, err := scanSQLiteModelConfig(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) UpsertModelConfig(projectRoot string, input UpsertModelConfigInput) (*ModelConfig, error) {
	now := time.Now().UTC()
	if input.APIKeyCiphertext == nil && input.APIKeyHint == nil {
		_, err := s.db.Exec(`
INSERT INTO codeflow_model_configs (project_root, provider, model, base_url, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_root) DO UPDATE SET
  provider=excluded.provider,
  model=excluded.model,
  base_url=excluded.base_url,
  updated_at=excluded.updated_at
`, projectRoot, input.Provider, input.Model, input.BaseURL, formatTS(now), formatTS(now))
		if err != nil {
			return nil, err
		}
		return s.GetModelConfig(projectRoot)
	}
	apiKeyCiphertext := ""
	apiKeyHint := ""
	if input.APIKeyCiphertext != nil {
		apiKeyCiphertext = *input.APIKeyCiphertext
	}
	if input.APIKeyHint != nil {
		apiKeyHint = *input.APIKeyHint
	}
	_, err := s.db.Exec(`
INSERT INTO codeflow_model_configs (project_root, provider, model, base_url, api_key_ciphertext, api_key_hint, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_root) DO UPDATE SET
  provider=excluded.provider,
  model=excluded.model,
  base_url=excluded.base_url,
  api_key_ciphertext=excluded.api_key_ciphertext,
  api_key_hint=excluded.api_key_hint,
  updated_at=excluded.updated_at
`, projectRoot, input.Provider, input.Model, input.BaseURL, apiKeyCiphertext, apiKeyHint, formatTS(now), formatTS(now))
	if err != nil {
		return nil, err
	}
	return s.GetModelConfig(projectRoot)
}

func (s *SQLiteSessionStore) Register(rootPath, name string, metadata map[string]string) (*workspace.Workspace, error) {
	now := time.Now().UTC()
	id := "ws_" + uuid.NewString()[:8]
	if existing, err := s.GetWorkspaceByRoot(rootPath); err != nil {
		return nil, err
	} else if existing != nil {
		_, err := s.db.Exec(`
UPDATE codeflow_workspaces
SET active=1, updated_at=?, last_opened_at=?
WHERE id=?
`, formatTS(now), formatTS(now), existing.ID)
		if err != nil {
			return nil, err
		}
		_, _ = s.db.Exec(`UPDATE codeflow_workspaces SET active=0 WHERE id<>?`, existing.ID)
		return s.GetWorkspaceByID(existing.ID)
	}
	encoded, _ := json.Marshal(metadata)
	configPath := metadata["config_path"]
	agentMDPath := metadata["agent_md_path"]
	defaultBranch := metadata["default_branch"]
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE codeflow_workspaces SET active=0`); err != nil {
		return nil, err
	}
	_, err = tx.Exec(`
INSERT INTO codeflow_workspaces (id, name, root_path, config_path, agent_md_path, default_branch, metadata, active, created_at, updated_at, last_opened_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
`, id, defaultTitle(name), rootPath, configPath, agentMDPath, defaultBranch, string(encoded), formatTS(now), formatTS(now), formatTS(now))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetWorkspaceByID(id)
}

func (s *SQLiteSessionStore) ListWorkspaces() ([]workspace.Workspace, error) {
	rows, err := s.db.Query(`
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
ORDER BY active DESC, last_opened_at DESC, created_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []workspace.Workspace{}
	for rows.Next() {
		item, err := scanSQLiteWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) GetWorkspaceByID(id string) (*workspace.Workspace, error) {
	row := s.db.QueryRow(`
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
WHERE id=?
`, id)
	item, err := scanSQLiteWorkspace(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) GetWorkspaceByRoot(rootPath string) (*workspace.Workspace, error) {
	row := s.db.QueryRow(`
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
WHERE root_path=?
`, rootPath)
	item, err := scanSQLiteWorkspace(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) GetCurrentWorkspace() (*workspace.Workspace, error) {
	row := s.db.QueryRow(`
SELECT id, name, root_path, config_path, agent_md_path, default_branch, metadata, active, created_at, updated_at, last_opened_at
FROM codeflow_workspaces
WHERE active=1
ORDER BY last_opened_at DESC
LIMIT 1
`)
	item, err := scanSQLiteWorkspace(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) SwitchWorkspace(id string) (*workspace.Workspace, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE codeflow_workspaces SET active=0`); err != nil {
		return nil, err
	}
	result, err := tx.Exec(`UPDATE codeflow_workspaces SET active=1, updated_at=?, last_opened_at=? WHERE id=?`, formatTS(now), formatTS(now), id)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetWorkspaceByID(id)
}

func (s *SQLiteSessionStore) RemoveWorkspace(id string) error {
	_, err := s.db.Exec(`DELETE FROM codeflow_workspaces WHERE id=?`, id)
	return err
}

type sqliteScanner interface {
	Scan(dest ...any) error
}

func scanSQLiteSession(row sqliteScanner) (*cfsession.Session, error) {
	var (
		item      cfsession.Session
		activeInt int
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&item.ID, &item.ProjectRoot, &item.Title, &item.AgentMD, &activeInt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	item.Active = activeInt == 1
	item.CreatedAt = parseTS(createdAt)
	item.UpdatedAt = parseTS(updatedAt)
	return &item, nil
}

func scanSQLiteApproval(row sqliteScanner) (*ApprovalRecord, error) {
	var (
		item      ApprovalRecord
		decidedAt sql.NullString
		createdAt string
		updatedAt string
	)
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
		&decidedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	if decidedAt.Valid {
		v := parseTS(decidedAt.String)
		item.DecidedAt = &v
	}
	item.CreatedAt = parseTS(createdAt)
	item.UpdatedAt = parseTS(updatedAt)
	return &item, nil
}

func scanSQLiteTaskEvent(row sqliteScanner) (*TaskEvent, error) {
	var (
		item      TaskEvent
		createdAt string
	)
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
		&createdAt,
	); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTS(createdAt)
	return &item, nil
}

func scanSQLiteMessage(row sqliteScanner) (*MessageRecord, error) {
	var (
		item      MessageRecord
		createdAt string
	)
	if err := row.Scan(
		&item.ID,
		&item.SessionID,
		&item.RequestID,
		&item.Role,
		&item.Content,
		&item.ToolCallID,
		&item.ToolName,
		&createdAt,
	); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTS(createdAt)
	return &item, nil
}

func scanSQLiteModelConfig(row sqliteScanner) (*ModelConfig, error) {
	var (
		item      ModelConfig
		createdAt string
		updatedAt string
	)
	if err := row.Scan(
		&item.ProjectRoot,
		&item.Provider,
		&item.Model,
		&item.BaseURL,
		&item.APIKeyCiphertext,
		&item.APIKeyHint,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTS(createdAt)
	item.UpdatedAt = parseTS(updatedAt)
	return &item, nil
}

func scanSQLiteWorkspace(row sqliteScanner) (*workspace.Workspace, error) {
	var (
		item         workspace.Workspace
		metadataJSON string
		activeInt    int
		createdAt    string
		updatedAt    string
		lastOpenedAt string
	)
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.RootPath,
		&item.ConfigPath,
		&item.AgentMDPath,
		&item.DefaultBranch,
		&metadataJSON,
		&activeInt,
		&createdAt,
		&updatedAt,
		&lastOpenedAt,
	); err != nil {
		return nil, err
	}
	item.Active = activeInt == 1
	item.CreatedAt = parseTS(createdAt)
	item.UpdatedAt = parseTS(updatedAt)
	item.LastOpenedAt = parseTS(lastOpenedAt)
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &item.Metadata)
	}
	return &item, nil
}

func formatTS(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTS(v string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, v); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}
