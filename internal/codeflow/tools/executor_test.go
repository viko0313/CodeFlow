package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/codeflow/internal/codeflow/permission"
)

func TestExecutorDeniedWriteDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	gate := permission.NewGate(permission.Options{Confirmer: func(context.Context, permission.Operation) (permission.Decision, error) {
		return permission.Decision{Allowed: false, Reason: "test denied"}, nil
	}})
	executor := NewExecutor(gate, nil)
	_, err := executor.Execute(context.Background(), Operation{Kind: permission.OperationWriteFile, ProjectRoot: dir, Path: "out.txt", Content: "nope"}, "s1")
	if err == nil {
		t.Fatal("expected denial error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("file should not exist after denied write: %v", statErr)
	}
}

func TestExecutorApprovedWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	var preview string
	gate := permission.NewGate(permission.Options{Confirmer: func(ctx context.Context, op permission.Operation) (permission.Decision, error) {
		preview = op.Preview
		return permission.Decision{Allowed: true, Reason: "ok"}, nil
	}})
	executor := NewExecutor(gate, nil)
	if _, err := executor.Execute(context.Background(), Operation{Kind: permission.OperationWriteFile, ProjectRoot: dir, Path: "out.txt", Content: "hello\n"}, "s1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview, "+++ after") || !strings.Contains(preview, "+hello") {
		t.Fatalf("expected diff preview, got:\n%s", preview)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("unexpected file content: %q", data)
	}
}
