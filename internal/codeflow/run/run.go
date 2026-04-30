package run

import "time"

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type AgentRun struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	PlanID        string    `json:"plan_id,omitempty"`
	Status        Status    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at,omitempty"`
	ModelProvider string    `json:"model_provider,omitempty"`
	ModelName     string    `json:"model_name,omitempty"`
	TotalTokens   int       `json:"total_tokens,omitempty"`
	TotalCost     string    `json:"total_cost,omitempty"`
	Error         string    `json:"error,omitempty"`
}

type EventType string

const (
	EventUserInput         EventType = "user_input"
	EventModelStart        EventType = "model_start"
	EventModelToken        EventType = "model_token"
	EventModelEnd          EventType = "model_end"
	EventToolStart         EventType = "tool_start"
	EventToolEnd           EventType = "tool_end"
	EventToolError         EventType = "tool_error"
	EventApprovalRequested EventType = "approval_requested"
	EventApprovalResult    EventType = "approval_result"
	EventPlanStepUpdate    EventType = "plan_step_update"
	EventCheckpointCreated EventType = "checkpoint_created"
	EventMemoryCompressed  EventType = "memory_compressed"
	EventError             EventType = "error"
)

type RunEvent struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
	LatencyMS int64          `json:"latency_ms,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}
