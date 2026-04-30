package storage

import "time"

type SessionSummary struct {
	SessionID   string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Summary     string    `json:"summary"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SummaryStore interface {
	GetSessionSummary(sessionID string) (*SessionSummary, error)
	UpsertSessionSummary(item SessionSummary) (*SessionSummary, error)
	ClearSessionSummary(sessionID string) error
}
