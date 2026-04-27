package storage

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type TraceEvent struct {
	SessionID    string         `json:"session_id"`
	RequestID    string         `json:"request_id"`
	SpanID       string         `json:"span_id,omitempty"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Iteration    int            `json:"iteration,omitempty"`
	ToolName     string         `json:"tool_name,omitempty"`
	ToolCallID   string         `json:"tool_call_id,omitempty"`
	EventType    string         `json:"event_type"`
	Status       string         `json:"status,omitempty"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
	ErrorType    string         `json:"error_type,omitempty"`
	CreatedAt    time.Time      `json:"created_at,omitempty"`
}

type EvalSummary struct {
	SessionID       string `json:"session_id"`
	Requests        int    `json:"requests"`
	ToolCalls       int    `json:"tool_calls"`
	ToolFailures    int    `json:"tool_failures"`
	Duplicates      int    `json:"duplicates"`
	TotalDurationMS int64  `json:"total_duration_ms"`
	FinalStatus     string `json:"final_status"`
}

type TraceStore interface {
	RecordTrace(ctx context.Context, event TraceEvent) error
	ListTrace(ctx context.Context, requestID string) ([]TraceEvent, error)
	SummarizeSession(ctx context.Context, sessionID string, limit int) (EvalSummary, error)
}

type TraceRecorder struct {
	events TaskEventStore
}

func NewTraceRecorder(events TaskEventStore) *TraceRecorder {
	return &TraceRecorder{events: events}
}

func (r *TraceRecorder) RecordTrace(ctx context.Context, event TraceEvent) error {
	_ = ctx
	if r == nil || r.events == nil {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(event.Status) == "" {
		event.Status = "ok"
	}
	payload, _ := json.Marshal(event)
	_, err := r.events.CreateTaskEvent(CreateTaskEventInput{
		ID:        traceEventID(event),
		SessionID: event.SessionID,
		RequestID: event.RequestID,
		Source:    "trace",
		Level:     traceLevel(event),
		EventType: "trace." + strings.TrimPrefix(event.EventType, "trace."),
		Message:   event.EventType,
		Payload:   string(payload),
		CreatedAt: event.CreatedAt,
	})
	return err
}

func (r *TraceRecorder) ListTrace(ctx context.Context, requestID string) ([]TraceEvent, error) {
	_ = ctx
	if r == nil || r.events == nil {
		return []TraceEvent{}, nil
	}
	items, err := r.events.ListTaskEvents(ListTaskEventsOptions{Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make([]TraceEvent, 0, len(items))
	for _, item := range items {
		if item.RequestID != requestID || !strings.HasPrefix(item.EventType, "trace.") {
			continue
		}
		if event, ok := parseTraceEvent(item); ok {
			out = append(out, event)
		}
	}
	reverseTrace(out)
	return out, nil
}

func (r *TraceRecorder) SummarizeSession(ctx context.Context, sessionID string, limit int) (EvalSummary, error) {
	_ = ctx
	if limit <= 0 || limit > 2000 {
		limit = 1000
	}
	summary := EvalSummary{SessionID: sessionID, FinalStatus: "unknown"}
	if r == nil || r.events == nil {
		return summary, nil
	}
	items, err := r.events.ListTaskEvents(ListTaskEventsOptions{SessionID: sessionID, Limit: limit})
	if err != nil {
		return summary, err
	}
	requests := map[string]bool{}
	for _, item := range items {
		if !strings.HasPrefix(item.EventType, "trace.") {
			continue
		}
		event, ok := parseTraceEvent(item)
		if !ok {
			continue
		}
		if event.RequestID != "" {
			requests[event.RequestID] = true
		}
		switch event.EventType {
		case "tool.call.completed", "tool.call.failed", "tool.call.warning":
			summary.ToolCalls++
		}
		if event.EventType == "tool.call.failed" {
			summary.ToolFailures++
		}
		if event.EventType == "tool.call.duplicate_detected" {
			summary.Duplicates++
		}
		if event.DurationMS > 0 {
			summary.TotalDurationMS += event.DurationMS
		}
		if event.EventType == "turn.completed" {
			summary.FinalStatus = "completed"
		}
		if event.EventType == "turn.failed" && summary.FinalStatus != "completed" {
			summary.FinalStatus = "failed"
		}
	}
	summary.Requests = len(requests)
	return summary, nil
}

func parseTraceEvent(item TaskEvent) (TraceEvent, bool) {
	var event TraceEvent
	if err := json.Unmarshal([]byte(item.Payload), &event); err != nil {
		return TraceEvent{}, false
	}
	event.EventType = strings.TrimPrefix(event.EventType, "trace.")
	if event.CreatedAt.IsZero() {
		event.CreatedAt = item.CreatedAt
	}
	return event, true
}

func traceLevel(event TraceEvent) string {
	switch event.Status {
	case "error":
		return "error"
	case "warning":
		return "warn"
	default:
		return "info"
	}
}

func traceEventID(event TraceEvent) string {
	sum := sha1.Sum([]byte(event.RequestID + event.EventType + event.SpanID + event.ToolCallID + event.CreatedAt.String()))
	return "trc_" + hex.EncodeToString(sum[:])[:16]
}

func reverseTrace(items []TraceEvent) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
