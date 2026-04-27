package storage

import (
	"context"
	"testing"
	"time"
)

func TestTraceRecorderUsesTaskEventsAndBuildsEvalSummary(t *testing.T) {
	store := &memoryTaskEventStore{}
	recorder := NewTraceRecorder(store)
	ctx := context.Background()
	events := []TraceEvent{
		{SessionID: "s1", RequestID: "r1", EventType: "turn.started", Status: "ok"},
		{SessionID: "s1", RequestID: "r1", EventType: "tool.call.completed", Status: "ok", ToolName: "read_file", DurationMS: 10},
		{SessionID: "s1", RequestID: "r1", EventType: "tool.call.failed", Status: "error", ToolName: "run_check", ErrorType: "command_failed", DurationMS: 20},
		{SessionID: "s1", RequestID: "r1", EventType: "tool.call.duplicate_detected", Status: "warning", ToolName: "read_file"},
		{SessionID: "s1", RequestID: "r1", EventType: "turn.failed", Status: "error", ErrorType: "budget_exhausted"},
	}
	for _, event := range events {
		if err := recorder.RecordTrace(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	trace, err := recorder.ListTrace(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(trace) != len(events) || trace[0].EventType != "turn.started" {
		t.Fatalf("unexpected trace events: %+v", trace)
	}
	summary, err := recorder.SummarizeSession(ctx, "s1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ToolCalls != 2 || summary.ToolFailures != 1 || summary.Duplicates != 1 || summary.FinalStatus != "failed" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.TotalDurationMS != 30 {
		t.Fatalf("unexpected duration: %+v", summary)
	}
}

type memoryTaskEventStore struct {
	events []TaskEvent
}

func (s *memoryTaskEventStore) CreateTaskEvent(input CreateTaskEventInput) (*TaskEvent, error) {
	item := TaskEvent{
		ID:          input.ID,
		SessionID:   input.SessionID,
		RequestID:   input.RequestID,
		OperationID: input.OperationID,
		ApprovalID:  input.ApprovalID,
		Source:      input.Source,
		Level:       input.Level,
		EventType:   input.EventType,
		Message:     input.Message,
		Payload:     input.Payload,
		CreatedAt:   input.CreatedAt,
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	s.events = append([]TaskEvent{item}, s.events...)
	copy := item
	return &copy, nil
}

func (s *memoryTaskEventStore) ListTaskEvents(opts ListTaskEventsOptions) ([]TaskEvent, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	out := make([]TaskEvent, 0, limit)
	for _, item := range s.events {
		if opts.SessionID != "" && item.SessionID != opts.SessionID {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
