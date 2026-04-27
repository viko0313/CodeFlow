package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
	cftools "github.com/viko0313/CodeFlow/internal/codeflow/tools"
)

type fakeToolModel struct {
	responses []*schema.Message
	calls     int
}

func (m *fakeToolModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	if m.calls >= len(m.responses) {
		return nil, errors.New("unexpected generate call")
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func (m *fakeToolModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not used")
}

func (m *fakeToolModel) BindTools(tools []*schema.ToolInfo) error {
	return nil
}

func (m *fakeToolModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

func TestAgentLoopFeedsToolResultBackToModel(t *testing.T) {
	registry := cftools.NewToolRegistry()
	if err := registry.Register(cftools.ToolSpec{
		Name:        "echo",
		Description: "echo test tool",
		Toolset:     cftools.DefaultToolset,
		Schema:      &schema.ToolInfo{Name: "echo", Desc: "echo test tool"},
		Handler: func(ctx context.Context, args json.RawMessage, runtime cftools.ToolRuntime) (cftools.ToolResult, error) {
			return cftools.ToolResult{Content: "tool said hi"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	model := &fakeToolModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_1", Function: schema.FunctionCall{Name: "echo", Arguments: `{}`}}}),
		schema.AssistantMessage("done", nil),
	}}
	engine := &LLMEngine{
		cfg:      &cfconfig.Config{Runtime: cfconfig.RuntimeConfig{MaxActions: 4, MaxContextTurns: 20}, Agent: cfconfig.AgentConfig{Mode: "react"}},
		model:    model,
		registry: registry,
		logger:   nilLogger(),
	}
	events, err := engine.Run(context.Background(), Request{SessionID: "s1", RequestID: "r1", ProjectRoot: t.TempDir(), Input: "use tool"})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for event := range events {
		if event.Type == EventOutput {
			output = event.Content
		}
		if event.Type == EventError {
			t.Fatalf("unexpected error event: %s", event.Content)
		}
	}
	if output != "done" {
		t.Fatalf("expected final output after tool loop, got %q", output)
	}
	if model.calls != 2 {
		t.Fatalf("expected two model calls, got %d", model.calls)
	}
}

func TestAgentLoopRunsCodingHarnessTools(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.go")
	if err := os.WriteFile(target, []byte("package main\nfunc main() { println(\"old\") }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gate := permission.NewGate(permission.Options{Confirmer: func(ctx context.Context, op permission.Operation) (permission.Decision, error) {
		return permission.Decision{Allowed: true, Reason: "test approved"}, nil
	}})
	executor := cftools.NewExecutor(gate, nil, nil, nil)
	model := &fakeToolModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_search", Function: schema.FunctionCall{Name: "search_code", Arguments: `{"query":"old"}`}}}),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_patch", Function: schema.FunctionCall{Name: "apply_patch", Arguments: `{"path":"main.go","old":"old","new":"new"}`}}}),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_check", Function: schema.FunctionCall{Name: "run_check", Arguments: `{"command":"go version","timeout_seconds":20}`}}}),
		schema.AssistantMessage("patched and checked", nil),
	}}
	engine := &LLMEngine{
		cfg:      &cfconfig.Config{Runtime: cfconfig.RuntimeConfig{MaxActions: 6, MaxContextTurns: 30}, Agent: cfconfig.AgentConfig{Mode: "react"}},
		model:    model,
		registry: cftools.DefaultRegistry(),
		executor: executor,
		logger:   nilLogger(),
	}
	events, err := engine.Run(context.Background(), Request{SessionID: "s1", RequestID: "r1", ProjectRoot: root, Input: "patch old to new and check"})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for event := range events {
		if event.Type == EventError {
			t.Fatalf("unexpected error event: %s", event.Content)
		}
		if event.Type == EventOutput {
			output = event.Content
		}
	}
	if output != "patched and checked" {
		t.Fatalf("unexpected output: %q", output)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "new") {
		t.Fatalf("patch did not update file: %s", string(data))
	}
}

func TestAgentLoopRecordsTraceAndDuplicateWarnings(t *testing.T) {
	traceStore := &fakeTraceStore{}
	registry := cftools.NewToolRegistry()
	if err := registry.Register(cftools.ToolSpec{
		Name:        "echo",
		Description: "echo test tool",
		Toolset:     cftools.DefaultToolset,
		Schema:      &schema.ToolInfo{Name: "echo", Desc: "echo test tool"},
		Handler: func(ctx context.Context, args json.RawMessage, runtime cftools.ToolRuntime) (cftools.ToolResult, error) {
			return cftools.ToolResult{Content: "ok"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	model := &fakeToolModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_1", Function: schema.FunctionCall{Name: "echo", Arguments: `{"x":1}`}}}),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_2", Function: schema.FunctionCall{Name: "echo", Arguments: `{"x":1}`}}}),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call_3", Function: schema.FunctionCall{Name: "echo", Arguments: `{"x":1}`}}}),
		schema.AssistantMessage("done", nil),
	}}
	engine := &LLMEngine{
		cfg:      &cfconfig.Config{Runtime: cfconfig.RuntimeConfig{MaxActions: 6, MaxContextTurns: 20}, Agent: cfconfig.AgentConfig{Mode: "react"}},
		model:    model,
		registry: registry,
		traces:   traceStore,
		logger:   nilLogger(),
	}
	events, err := engine.Run(context.Background(), Request{SessionID: "s1", RequestID: "r1", ProjectRoot: t.TempDir(), Input: "repeat tool"})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Type == EventError {
			t.Fatalf("unexpected error event: %s", event.Content)
		}
	}
	if !traceStore.has("turn.started") || !traceStore.has("llm.iteration.started") || !traceStore.has("tool.call.started") || !traceStore.has("turn.completed") {
		t.Fatalf("trace missing expected lifecycle events: %+v", traceStore.events)
	}
	if traceStore.count("tool.call.duplicate_detected") != 2 {
		t.Fatalf("expected duplicate trace events for second and third call, got %+v", traceStore.events)
	}
	if traceStore.count("tool.call.warning") != 1 {
		t.Fatalf("expected third duplicate to be soft-blocked with warning, got %+v", traceStore.events)
	}
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeTraceStore struct {
	events []storage.TraceEvent
}

func (s *fakeTraceStore) RecordTrace(ctx context.Context, event storage.TraceEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *fakeTraceStore) ListTrace(ctx context.Context, requestID string) ([]storage.TraceEvent, error) {
	var out []storage.TraceEvent
	for _, event := range s.events {
		if event.RequestID == requestID {
			out = append(out, event)
		}
	}
	return out, nil
}

func (s *fakeTraceStore) SummarizeSession(ctx context.Context, sessionID string, limit int) (storage.EvalSummary, error) {
	return storage.EvalSummary{}, nil
}

func (s *fakeTraceStore) has(eventType string) bool {
	return s.count(eventType) > 0
}

func (s *fakeTraceStore) count(eventType string) int {
	count := 0
	for _, event := range s.events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}
