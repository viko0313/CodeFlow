package storage

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
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
`)
	if err != nil {
		return fmt.Errorf("migrate CodeFlow session schema: %w", err)
	}
	return nil
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
	tag, err := s.pool.Exec(s.ctx, `DELETE FROM codeflow_sessions WHERE project_root=$1 AND id=$2`, projectRoot, sessionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
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
