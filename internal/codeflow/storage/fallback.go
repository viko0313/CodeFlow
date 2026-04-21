package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
)

const (
	BackendPostgres = "postgres"
	BackendSQLite   = "sqlite"
)

func OpenSessionStoreWithFallback(ctx context.Context, postgresDSN, dataDir string) (cfsession.Store, string, bool, error) {
	postgresDSN = strings.TrimSpace(postgresDSN)
	if postgresDSN != "" {
		pgStore, err := NewPostgresSessionStore(ctx, postgresDSN)
		if err == nil {
			return pgStore, BackendPostgres, false, nil
		}
		sqliteStore, sqliteErr := NewSQLiteSessionStore(filepath.Join(dataDir, "codeflow.db"))
		if sqliteErr != nil {
			return nil, "", false, fmt.Errorf("connect postgres: %w; fallback sqlite failed: %v", err, sqliteErr)
		}
		return sqliteStore, BackendSQLite, true, nil
	}
	sqliteStore, err := NewSQLiteSessionStore(filepath.Join(dataDir, "codeflow.db"))
	if err != nil {
		return nil, "", false, err
	}
	return sqliteStore, BackendSQLite, true, nil
}
