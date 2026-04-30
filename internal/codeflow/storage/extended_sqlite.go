package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/viko0313/CodeFlow/internal/codeflow/checkpoint"
	"github.com/viko0313/CodeFlow/internal/codeflow/plan"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
)

func (s *SQLiteSessionStore) CreateRunRecord(record run.AgentRun) (*run.AgentRun, error) {
	if strings.TrimSpace(record.ID) == "" {
		record.ID = "run_" + uuid.NewString()[:8]
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now().UTC()
	}
	endedAt := ""
	if !record.EndedAt.IsZero() {
		endedAt = formatTS(record.EndedAt)
	}
	_, err := s.db.Exec(`INSERT INTO codeflow_runs (id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.SessionID, record.WorkspaceID, record.PlanID, string(record.Status), formatTS(record.StartedAt), endedAt, record.ModelProvider, record.ModelName, record.TotalTokens, record.TotalCost, record.Error)
	if err != nil {
		return nil, err
	}
	return s.GetRunRecord(record.ID)
}

func (s *SQLiteSessionStore) UpdateRunRecord(record run.AgentRun) (*run.AgentRun, error) {
	endedAt := ""
	if !record.EndedAt.IsZero() {
		endedAt = formatTS(record.EndedAt)
	}
	_, err := s.db.Exec(`UPDATE codeflow_runs SET status=?, ended_at=?, model_provider=?, model_name=?, total_tokens=?, total_cost=?, error=? WHERE id=?`,
		string(record.Status), endedAt, record.ModelProvider, record.ModelName, record.TotalTokens, record.TotalCost, record.Error, record.ID)
	if err != nil {
		return nil, err
	}
	return s.GetRunRecord(record.ID)
}

func (s *SQLiteSessionStore) ListRunRecords(sessionID, workspaceID string, limit int) ([]run.AgentRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error FROM codeflow_runs`
	args := []any{}
	where := []string{}
	if sessionID != "" {
		where = append(where, "session_id=?")
		args = append(args, sessionID)
	}
	if workspaceID != "" {
		where = append(where, "workspace_id=?")
		args = append(args, workspaceID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY started_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []run.AgentRun{}
	for rows.Next() {
		item, err := scanSQLiteRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) GetRunRecord(id string) (*run.AgentRun, error) {
	row := s.db.QueryRow(`SELECT id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error FROM codeflow_runs WHERE id=?`, id)
	item, err := scanSQLiteRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) CreateRunEventRecord(event run.RunEvent) (*run.RunEvent, error) {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "runevt_" + uuid.NewString()[:8]
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	payload, _ := json.Marshal(event.Payload)
	_, err := s.db.Exec(`INSERT INTO codeflow_run_events (id, run_id, type, timestamp, payload, latency_ms, request_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.RunID, string(event.Type), formatTS(event.Timestamp), string(payload), event.LatencyMS, event.RequestID)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *SQLiteSessionStore) ListRunEventRecords(runID string, limit int) ([]run.RunEvent, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.Query(`SELECT id, run_id, type, timestamp, payload, latency_ms, request_id FROM codeflow_run_events WHERE run_id=? ORDER BY timestamp ASC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []run.RunEvent{}
	for rows.Next() {
		item, err := scanSQLiteRunEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) CreatePlanRecord(item plan.Plan) (*plan.Plan, error) {
	if strings.TrimSpace(item.ID) == "" {
		item.ID = "plan_" + uuid.NewString()[:8]
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	pref, _ := json.Marshal(item.Preference)
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO codeflow_plans (id, session_id, workspace_id, goal, status, preference, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.SessionID, item.WorkspaceID, item.Goal, string(item.Status), string(pref), formatTS(item.CreatedAt), formatTS(item.UpdatedAt))
	if err != nil {
		return nil, err
	}
	for i := range item.Steps {
		step := item.Steps[i]
		if strings.TrimSpace(step.ID) == "" {
			step.ID = "step_" + uuid.NewString()[:8]
		}
		step.PlanID = item.ID
		step.Position = i
		item.Steps[i] = step
		if err := insertSQLitePlanStep(tx, step); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPlanRecord(item.ID)
}

func (s *SQLiteSessionStore) GetPlanRecord(id string) (*plan.Plan, error) {
	row := s.db.QueryRow(`SELECT id, session_id, workspace_id, goal, status, preference, created_at, updated_at FROM codeflow_plans WHERE id=?`, id)
	item, err := scanSQLitePlan(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	steps, err := s.listPlanSteps(id)
	if err != nil {
		return nil, err
	}
	item.Steps = steps
	return item, nil
}

func (s *SQLiteSessionStore) ListPlanRecords(sessionID, workspaceID string, limit int) ([]plan.Plan, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := `SELECT id, session_id, workspace_id, goal, status, preference, created_at, updated_at FROM codeflow_plans`
	args := []any{}
	where := []string{}
	if sessionID != "" {
		where = append(where, "session_id=?")
		args = append(args, sessionID)
	}
	if workspaceID != "" {
		where = append(where, "workspace_id=?")
		args = append(args, workspaceID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.Plan{}
	for rows.Next() {
		item, err := scanSQLitePlan(rows)
		if err != nil {
			return nil, err
		}
		steps, err := s.listPlanSteps(item.ID)
		if err != nil {
			return nil, err
		}
		item.Steps = steps
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) UpdatePlanRecord(item plan.Plan) (*plan.Plan, error) {
	item.UpdatedAt = time.Now().UTC()
	pref, _ := json.Marshal(item.Preference)
	_, err := s.db.Exec(`UPDATE codeflow_plans SET goal=?, status=?, preference=?, updated_at=? WHERE id=?`,
		item.Goal, string(item.Status), string(pref), formatTS(item.UpdatedAt), item.ID)
	if err != nil {
		return nil, err
	}
	return s.GetPlanRecord(item.ID)
}

func (s *SQLiteSessionStore) UpdatePlanStepRecord(step plan.PlanStep) (*plan.PlanStep, error) {
	files, _ := json.Marshal(step.RelatedFiles)
	tools, _ := json.Marshal(step.ToolCalls)
	_, err := s.db.Exec(`UPDATE codeflow_plan_steps SET title=?, description=?, type=?, status=?, requires_approval=?, related_files=?, tool_calls=?, result_summary=?, error=?, position=? WHERE id=?`,
		step.Title, step.Description, string(step.Type), string(step.Status), boolToInt(step.RequiresApproval), string(files), string(tools), step.ResultSummary, step.Error, step.Position, step.ID)
	if err != nil {
		return nil, err
	}
	return &step, nil
}

func (s *SQLiteSessionStore) CreatePlanEventRecord(event plan.Event) (*plan.Event, error) {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "planevt_" + uuid.NewString()[:8]
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	payload, _ := json.Marshal(event.Payload)
	_, err := s.db.Exec(`INSERT INTO codeflow_plan_events (id, plan_id, step_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		event.ID, event.PlanID, event.StepID, event.Type, string(payload), formatTS(event.CreatedAt))
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *SQLiteSessionStore) ListPlanEventRecords(planID string, limit int) ([]plan.Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id, plan_id, step_id, type, payload, created_at FROM codeflow_plan_events WHERE plan_id=? ORDER BY created_at DESC LIMIT ?`, planID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.Event{}
	for rows.Next() {
		item, err := scanSQLitePlanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) CreateCheckpointRecord(item checkpoint.Checkpoint) (*checkpoint.Checkpoint, error) {
	if strings.TrimSpace(item.ID) == "" {
		item.ID = "ckpt_" + uuid.NewString()[:8]
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	filesJSON, _ := json.Marshal(item.ChangedFiles)
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO codeflow_checkpoints (id, workspace_id, session_id, run_id, plan_step_id, created_at, reason, git_head, changed_files, snapshot_path, patch_path, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.WorkspaceID, item.SessionID, item.RunID, item.PlanStepID, formatTS(item.CreatedAt), item.Reason, item.GitHead, string(filesJSON), item.SnapshotPath, item.PatchPath, item.Description)
	if err != nil {
		return nil, err
	}
	for i := range item.Files {
		file := item.Files[i]
		if strings.TrimSpace(file.ID) == "" {
			file.ID = "ckpf_" + uuid.NewString()[:8]
		}
		file.CheckpointID = item.ID
		if file.CreatedAt.IsZero() {
			file.CreatedAt = item.CreatedAt
		}
		_, err = tx.Exec(`INSERT INTO codeflow_checkpoint_files (id, checkpoint_id, path, existed, is_binary, size_bytes, sha256, content, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			file.ID, item.ID, file.Path, boolToInt(file.Existed), boolToInt(file.IsBinary), file.SizeBytes, file.SHA256, file.Content, formatTS(file.CreatedAt))
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetCheckpointRecord(item.ID)
}

func (s *SQLiteSessionStore) GetCheckpointRecord(id string) (*checkpoint.Checkpoint, error) {
	row := s.db.QueryRow(`SELECT id, workspace_id, session_id, run_id, plan_step_id, created_at, reason, git_head, changed_files, snapshot_path, patch_path, description FROM codeflow_checkpoints WHERE id=?`, id)
	item, err := scanSQLiteCheckpoint(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files, err := s.listCheckpointFiles(id)
	if err != nil {
		return nil, err
	}
	item.Files = files
	return item, nil
}

func (s *SQLiteSessionStore) ListCheckpointRecords(sessionID, workspaceID string, limit int) ([]checkpoint.Checkpoint, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := `SELECT id, workspace_id, session_id, run_id, plan_step_id, created_at, reason, git_head, changed_files, snapshot_path, patch_path, description FROM codeflow_checkpoints`
	args := []any{}
	where := []string{}
	if sessionID != "" {
		where = append(where, "session_id=?")
		args = append(args, sessionID)
	}
	if workspaceID != "" {
		where = append(where, "workspace_id=?")
		args = append(args, workspaceID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []checkpoint.Checkpoint{}
	for rows.Next() {
		item, err := scanSQLiteCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) GetSessionSummary(sessionID string) (*SessionSummary, error) {
	row := s.db.QueryRow(`SELECT session_id, workspace_id, summary, updated_at FROM codeflow_session_summaries WHERE session_id=?`, sessionID)
	item, err := scanSQLiteSummary(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *SQLiteSessionStore) UpsertSessionSummary(item SessionSummary) (*SessionSummary, error) {
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
INSERT INTO codeflow_session_summaries (session_id, workspace_id, summary, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET workspace_id=excluded.workspace_id, summary=excluded.summary, updated_at=excluded.updated_at
`, item.SessionID, item.WorkspaceID, item.Summary, formatTS(item.UpdatedAt))
	if err != nil {
		return nil, err
	}
	return s.GetSessionSummary(item.SessionID)
}

func (s *SQLiteSessionStore) ClearSessionSummary(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM codeflow_session_summaries WHERE session_id=?`, sessionID)
	return err
}

func insertSQLitePlanStep(tx *sql.Tx, step plan.PlanStep) error {
	files, _ := json.Marshal(step.RelatedFiles)
	tools, _ := json.Marshal(step.ToolCalls)
	_, err := tx.Exec(`INSERT INTO codeflow_plan_steps (id, plan_id, title, description, type, status, requires_approval, related_files, tool_calls, result_summary, error, position) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.PlanID, step.Title, step.Description, string(step.Type), string(step.Status), boolToInt(step.RequiresApproval), string(files), string(tools), step.ResultSummary, step.Error, step.Position)
	return err
}

func (s *SQLiteSessionStore) listPlanSteps(planID string) ([]plan.PlanStep, error) {
	rows, err := s.db.Query(`SELECT id, plan_id, title, description, type, status, requires_approval, related_files, tool_calls, result_summary, error, position FROM codeflow_plan_steps WHERE plan_id=? ORDER BY position ASC`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.PlanStep{}
	for rows.Next() {
		item, err := scanSQLitePlanStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *SQLiteSessionStore) listCheckpointFiles(checkpointID string) ([]checkpoint.FileSnapshot, error) {
	rows, err := s.db.Query(`SELECT id, checkpoint_id, path, existed, is_binary, size_bytes, sha256, content, created_at FROM codeflow_checkpoint_files WHERE checkpoint_id=? ORDER BY created_at ASC`, checkpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []checkpoint.FileSnapshot{}
	for rows.Next() {
		item, err := scanSQLiteCheckpointFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanSQLiteRun(row sqliteScanner) (*run.AgentRun, error) {
	var item run.AgentRun
	var startedAt, endedAt string
	if err := row.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.PlanID, &item.Status, &startedAt, &endedAt, &item.ModelProvider, &item.ModelName, &item.TotalTokens, &item.TotalCost, &item.Error); err != nil {
		return nil, err
	}
	item.StartedAt = parseTS(startedAt)
	if endedAt != "" {
		item.EndedAt = parseTS(endedAt)
	}
	return &item, nil
}

func scanSQLiteRunEvent(row sqliteScanner) (*run.RunEvent, error) {
	var item run.RunEvent
	var payloadJSON, ts string
	if err := row.Scan(&item.ID, &item.RunID, &item.Type, &ts, &payloadJSON, &item.LatencyMS, &item.RequestID); err != nil {
		return nil, err
	}
	item.Timestamp = parseTS(ts)
	_ = json.Unmarshal([]byte(payloadJSON), &item.Payload)
	return &item, nil
}

func scanSQLitePlan(row sqliteScanner) (*plan.Plan, error) {
	var item plan.Plan
	var prefJSON, createdAt, updatedAt string
	if err := row.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.Goal, &item.Status, &prefJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTS(createdAt)
	item.UpdatedAt = parseTS(updatedAt)
	_ = json.Unmarshal([]byte(prefJSON), &item.Preference)
	return &item, nil
}

func scanSQLitePlanStep(row sqliteScanner) (*plan.PlanStep, error) {
	var item plan.PlanStep
	var filesJSON, toolsJSON string
	var req int
	if err := row.Scan(&item.ID, &item.PlanID, &item.Title, &item.Description, &item.Type, &item.Status, &req, &filesJSON, &toolsJSON, &item.ResultSummary, &item.Error, &item.Position); err != nil {
		return nil, err
	}
	item.RequiresApproval = req == 1
	_ = json.Unmarshal([]byte(filesJSON), &item.RelatedFiles)
	_ = json.Unmarshal([]byte(toolsJSON), &item.ToolCalls)
	return &item, nil
}

func scanSQLitePlanEvent(row sqliteScanner) (*plan.Event, error) {
	var item plan.Event
	var payloadJSON, createdAt string
	if err := row.Scan(&item.ID, &item.PlanID, &item.StepID, &item.Type, &payloadJSON, &createdAt); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTS(createdAt)
	_ = json.Unmarshal([]byte(payloadJSON), &item.Payload)
	return &item, nil
}

func scanSQLiteCheckpoint(row sqliteScanner) (*checkpoint.Checkpoint, error) {
	var item checkpoint.Checkpoint
	var changedJSON, createdAt string
	if err := row.Scan(&item.ID, &item.WorkspaceID, &item.SessionID, &item.RunID, &item.PlanStepID, &createdAt, &item.Reason, &item.GitHead, &changedJSON, &item.SnapshotPath, &item.PatchPath, &item.Description); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTS(createdAt)
	_ = json.Unmarshal([]byte(changedJSON), &item.ChangedFiles)
	return &item, nil
}

func scanSQLiteCheckpointFile(row sqliteScanner) (*checkpoint.FileSnapshot, error) {
	var item checkpoint.FileSnapshot
	var existed, isBinary int
	var createdAt string
	if err := row.Scan(&item.ID, &item.CheckpointID, &item.Path, &existed, &isBinary, &item.SizeBytes, &item.SHA256, &item.Content, &createdAt); err != nil {
		return nil, err
	}
	item.Existed = existed == 1
	item.IsBinary = isBinary == 1
	item.CreatedAt = parseTS(createdAt)
	return &item, nil
}

func scanSQLiteSummary(row sqliteScanner) (*SessionSummary, error) {
	var item SessionSummary
	var updatedAt string
	if err := row.Scan(&item.SessionID, &item.WorkspaceID, &item.Summary, &updatedAt); err != nil {
		return nil, err
	}
	item.UpdatedAt = parseTS(updatedAt)
	return &item, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
