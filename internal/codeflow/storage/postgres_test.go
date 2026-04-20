package storage

import (
	"context"
	"os"
	"testing"
)

func TestPostgresStoreRequiresDSN(t *testing.T) {
	if _, err := NewPostgresSessionStore(context.Background(), ""); err == nil {
		t.Fatal("expected missing dsn error")
	}
}

func TestPostgresSessionStoreIntegration(t *testing.T) {
	dsn := os.Getenv("CODEFLOW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CODEFLOW_TEST_POSTGRES_DSN to run integration test")
	}
	store, err := NewPostgresSessionStore(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.Create(t.TempDir(), "test", "rules")
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || !session.Active {
		t.Fatalf("bad session: %+v", session)
	}
}
