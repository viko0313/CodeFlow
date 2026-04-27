package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestRegistryRejectsDuplicateAndDispatchesUnknownAsStructuredResult(t *testing.T) {
	registry := NewToolRegistry()
	spec := ToolSpec{
		Name:    "echo",
		Toolset: DefaultToolset,
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	}
	if err := registry.Register(spec); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(spec); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	result, err := registry.Dispatch(context.Background(), schema.ToolCall{Function: schema.FunctionCall{Name: "missing"}}, ToolRuntime{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "unknown tool") {
		t.Fatalf("expected structured unknown tool result, got %q", result.Content)
	}
}

func TestTodoToolUpsertAndList(t *testing.T) {
	store := NewTodoStore()
	spec := TodoToolSpec()
	result, err := spec.Handler(context.Background(), json.RawMessage(`{"action":"upsert","content":"ship registry","status":"in_progress"}`), ToolRuntime{Todos: store})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "ship registry") {
		t.Fatalf("unexpected upsert result: %s", result.Content)
	}
	result, err = spec.Handler(context.Background(), json.RawMessage(`{"action":"list"}`), ToolRuntime{Todos: store})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "in_progress") {
		t.Fatalf("unexpected list result: %s", result.Content)
	}
}
