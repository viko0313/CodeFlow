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

	cfsession "github.com/cloudwego/codeflow/internal/codeflow/session"
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
