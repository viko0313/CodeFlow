package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/viko0313/CodeFlow/internal/codeflow/observability"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
)

type Compressor struct {
	summaries SummaryStore
	runs      *run.Recorder
}

type SummaryStore interface {
	GetSessionSummary(sessionID string) (*storage.SessionSummary, error)
	UpsertSessionSummary(item storage.SessionSummary) (*storage.SessionSummary, error)
}

func NewCompressor(summaries SummaryStore, runs *run.Recorder) *Compressor {
	return &Compressor{summaries: summaries, runs: runs}
}

func (c *Compressor) Compress(ctx context.Context, sessionID, workspaceID string, recent []storage.MessageRecord) (*storage.SessionSummary, error) {
	if c == nil || c.summaries == nil {
		return nil, fmt.Errorf("summary store is not configured")
	}
	summary := buildSummaryText(recent)
	item, err := c.summaries.UpsertSessionSummary(storage.SessionSummary{
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
		Summary:     summary,
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	runID := observability.RunIDFromContext(ctx)
	if c.runs != nil && runID != "" {
		_ = c.runs.Event(ctx, run.RunEvent{RunID: runID, Type: run.EventMemoryCompressed, Timestamp: time.Now().UTC(), RequestID: observability.RequestIDFromContext(ctx), Payload: map[string]any{"session_id": sessionID}})
	}
	return item, nil
}

func buildSummaryText(recent []storage.MessageRecord) string {
	if len(recent) == 0 {
		return ""
	}
	var userGoal, lastFailure string
	files := []string{}
	decisions := []string{}
	for _, msg := range recent {
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "user":
			if userGoal == "" {
				userGoal = msg.Content
			}
		case "assistant":
			if strings.Contains(strings.ToLower(msg.Content), "failed") {
				lastFailure = msg.Content
			}
		case "tool":
			if msg.ToolName != "" {
				decisions = append(decisions, msg.ToolName+": "+truncateSummary(msg.Content, 120))
			}
		}
		if msg.ToolName == "write_file" && msg.Content != "" {
			files = append(files, truncateSummary(msg.Content, 80))
		}
	}
	parts := []string{}
	if userGoal != "" {
		parts = append(parts, "Current task goal: "+truncateSummary(userGoal, 240))
	}
	if len(files) > 0 {
		parts = append(parts, "Recent file changes: "+strings.Join(files, "; "))
	}
	if len(decisions) > 0 {
		parts = append(parts, "Recent tool observations: "+strings.Join(decisions, "; "))
	}
	if lastFailure != "" {
		parts = append(parts, "Recent failure: "+truncateSummary(lastFailure, 240))
	}
	return strings.Join(parts, "\n")
}

func truncateSummary(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
