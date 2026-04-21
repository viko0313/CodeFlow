package approval

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
)

func TestServiceRejectRequiresReason(t *testing.T) {
	store := mustSQLiteStore(t)
	defer store.Close()
	service := NewService(store)
	record, err := store.CreateApproval(storage.CreateApprovalInput{
		OperationID: "op_1",
		Kind:        "shell",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Decide(record.ID, false, "")
	if err == nil {
		t.Fatal("expected reject reason error")
	}
	if err != ErrRejectReasonRequired {
		t.Fatalf("expected ErrRejectReasonRequired, got %v", err)
	}
}

func TestServiceDecideConflict(t *testing.T) {
	store := mustSQLiteStore(t)
	defer store.Close()
	service := NewService(store)
	record, err := store.CreateApproval(storage.CreateApprovalInput{
		OperationID: "op_2",
		Kind:        "shell",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Decide(record.ID, true, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Decide(record.ID, false, "nope"); err != ErrApprovalAlreadyDecided {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestServiceWaitForDecision(t *testing.T) {
	store := mustSQLiteStore(t)
	defer store.Close()
	service := NewService(store)
	record, err := store.CreateApproval(storage.CreateApprovalInput{
		OperationID: "op_3",
		Kind:        "write_file",
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = service.Decide(record.ID, true, "ok")
	}()
	allowed, reason, err := service.WaitForDecision(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed || reason != "ok" {
		t.Fatalf("unexpected wait result: allowed=%v reason=%q", allowed, reason)
	}
}

func mustSQLiteStore(t *testing.T) *storage.SQLiteSessionStore {
	t.Helper()
	store, err := storage.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "codeflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}
