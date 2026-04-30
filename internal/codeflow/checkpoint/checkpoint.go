package checkpoint

import "time"

type Checkpoint struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspace_id,omitempty"`
	SessionID    string         `json:"session_id,omitempty"`
	RunID        string         `json:"run_id,omitempty"`
	PlanStepID   string         `json:"plan_step_id,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	Reason       string         `json:"reason,omitempty"`
	GitHead      string         `json:"git_head,omitempty"`
	ChangedFiles []string       `json:"changed_files,omitempty"`
	SnapshotPath string         `json:"snapshot_path,omitempty"`
	PatchPath    string         `json:"patch_path,omitempty"`
	Description  string         `json:"description,omitempty"`
	Files        []FileSnapshot `json:"files,omitempty"`
}

type FileSnapshot struct {
	ID           string    `json:"id"`
	CheckpointID string    `json:"checkpoint_id,omitempty"`
	Path         string    `json:"path"`
	Existed      bool      `json:"existed"`
	IsBinary     bool      `json:"is_binary"`
	SizeBytes    int64     `json:"size_bytes"`
	SHA256       string    `json:"sha256,omitempty"`
	Content      string    `json:"content,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
