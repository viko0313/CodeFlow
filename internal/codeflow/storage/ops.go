package storage

import "time"

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
)

type ApprovalRecord struct {
	ID             string         `json:"id"`
	OperationID    string         `json:"operation_id"`
	SessionID      string         `json:"session_id"`
	ProjectRoot    string         `json:"project_root"`
	Kind           string         `json:"kind"`
	Path           string         `json:"path,omitempty"`
	Command        string         `json:"command,omitempty"`
	Preview        string         `json:"preview,omitempty"`
	Risk           string         `json:"risk,omitempty"`
	Timeout        string         `json:"timeout,omitempty"`
	RequestID      string         `json:"request_id,omitempty"`
	Status         ApprovalStatus `json:"status"`
	DecisionReason string         `json:"decision_reason,omitempty"`
	DecidedAt      *time.Time     `json:"decided_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type CreateApprovalInput struct {
	ID          string
	OperationID string
	SessionID   string
	ProjectRoot string
	Kind        string
	Path        string
	Command     string
	Preview     string
	Risk        string
	Timeout     string
	RequestID   string
}

type ListApprovalsOptions struct {
	Status string
	Limit  int
}

type ApprovalStore interface {
	CreateApproval(input CreateApprovalInput) (*ApprovalRecord, error)
	GetApproval(id string) (*ApprovalRecord, error)
	GetApprovalByOperationID(operationID string) (*ApprovalRecord, error)
	ListApprovals(opts ListApprovalsOptions) ([]ApprovalRecord, error)
	DecideApproval(id string, allowed bool, reason string) (*ApprovalRecord, error)
}

type TaskEvent struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	OperationID string    `json:"operation_id,omitempty"`
	ApprovalID  string    `json:"approval_id,omitempty"`
	Source      string    `json:"source"`
	Level       string    `json:"level"`
	EventType   string    `json:"event_type"`
	Message     string    `json:"message"`
	Payload     string    `json:"payload,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateTaskEventInput struct {
	ID          string
	SessionID   string
	RequestID   string
	OperationID string
	ApprovalID  string
	Source      string
	Level       string
	EventType   string
	Message     string
	Payload     string
	CreatedAt   time.Time
}

type ListTaskEventsOptions struct {
	SessionID string
	Limit     int
}

type TaskEventStore interface {
	CreateTaskEvent(input CreateTaskEventInput) (*TaskEvent, error)
	ListTaskEvents(opts ListTaskEventsOptions) ([]TaskEvent, error)
}
