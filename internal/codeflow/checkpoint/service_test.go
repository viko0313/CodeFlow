package checkpoint_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/viko0313/CodeFlow/internal/codeflow/checkpoint"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
)

func TestCheckpointCreateAndRewind(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "checkpoints.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := checkpoint.NewService(store, nil)
	item, err := svc.CreateForWrite(context.Background(), "ws1", "s1", "r1", "", root, []string{"demo.txt"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := svc.Rewind(context.Background(), root, item); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("expected rewind to restore original content, got %q", string(data))
	}
}
