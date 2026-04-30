package plan

import "time"

type Status string

const (
	StatusPlanning        Status = "planning"
	StatusWaitingApproval Status = "waiting_approval"
	StatusActing          Status = "acting"
	StatusPaused          Status = "paused"
	StatusCompleted       Status = "completed"
	StatusFailed          Status = "failed"
	StatusCancelled       Status = "cancelled"
)

type StepType string

const (
	StepRead    StepType = "read"
	StepSearch  StepType = "search"
	StepEdit    StepType = "edit"
	StepShell   StepType = "shell"
	StepTest    StepType = "test"
	StepReview  StepType = "review"
	StepSummary StepType = "summary"
)

type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

type Plan struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	Goal        string         `json:"goal"`
	Status      Status         `json:"status"`
	Steps       []PlanStep     `json:"steps"`
	Preference  PlanPreference `json:"preference"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type PlanStep struct {
	ID               string     `json:"id"`
	PlanID           string     `json:"plan_id,omitempty"`
	Title            string     `json:"title"`
	Description      string     `json:"description,omitempty"`
	Type             StepType   `json:"type"`
	Status           StepStatus `json:"status"`
	RequiresApproval bool       `json:"requires_approval"`
	RelatedFiles     []string   `json:"related_files,omitempty"`
	ToolCalls        []string   `json:"tool_calls,omitempty"`
	ResultSummary    string     `json:"result_summary,omitempty"`
	Error            string     `json:"error,omitempty"`
	Position         int        `json:"position"`
}

type PlanPreference struct {
	ConfirmBeforeWrite  bool `json:"confirm_before_write"`
	ConfirmBeforeShell  bool `json:"confirm_before_shell"`
	AutoRunTests        bool `json:"auto_run_tests"`
	AllowNetwork        bool `json:"allow_network"`
	MaxSteps            int  `json:"max_steps"`
	ResumeFromLastPause bool `json:"resume_from_last_pause"`
}

type Event struct {
	ID        string         `json:"id"`
	PlanID    string         `json:"plan_id"`
	StepID    string         `json:"step_id,omitempty"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
