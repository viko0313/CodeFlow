package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/viko0313/CodeFlow/internal/codeflow/checkpoint"
	"github.com/viko0313/CodeFlow/internal/codeflow/plan"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
)

func (s *PostgresSessionStore) CreateRunRecord(record run.AgentRun) (*run.AgentRun, error) {
	if strings.TrimSpace(record.ID) == "" {
		record.ID = "run_" + uuid.NewString()[:8]
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now().UTC()
	}
	row := s.pool.QueryRow(s.ctx, `INSERT INTO codeflow_runs (id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error`,
		record.ID, record.SessionID, record.WorkspaceID, record.PlanID, string(record.Status), record.StartedAt, zeroTime(record.EndedAt), record.ModelProvider, record.ModelName, record.TotalTokens, record.TotalCost, record.Error)
	return scanRun(row)
}

func (s *PostgresSessionStore) UpdateRunRecord(record run.AgentRun) (*run.AgentRun, error) {
	row := s.pool.QueryRow(s.ctx, `UPDATE codeflow_runs SET status=$2, ended_at=$3, model_provider=$4, model_name=$5, total_tokens=$6, total_cost=$7, error=$8 WHERE id=$1 RETURNING id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error`,
		record.ID, string(record.Status), zeroTime(record.EndedAt), record.ModelProvider, record.ModelName, record.TotalTokens, record.TotalCost, record.Error)
	return scanRun(row)
}

func (s *PostgresSessionStore) ListRunRecords(sessionID, workspaceID string, limit int) ([]run.AgentRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(s.ctx, `SELECT id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error FROM codeflow_runs WHERE ($1='' OR session_id=$1) AND ($2='' OR workspace_id=$2) ORDER BY started_at DESC LIMIT $3`, sessionID, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []run.AgentRun{}
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) GetRunRecord(id string) (*run.AgentRun, error) {
	row := s.pool.QueryRow(s.ctx, `SELECT id, session_id, workspace_id, plan_id, status, started_at, ended_at, model_provider, model_name, total_tokens, total_cost, error FROM codeflow_runs WHERE id=$1`, id)
	item, err := scanRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *PostgresSessionStore) CreateRunEventRecord(event run.RunEvent) (*run.RunEvent, error) {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "runevt_" + uuid.NewString()[:8]
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	payload, _ := json.Marshal(event.Payload)
	row := s.pool.QueryRow(s.ctx, `INSERT INTO codeflow_run_events (id, run_id, type, timestamp, payload, latency_ms, request_id) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7) RETURNING id, run_id, type, timestamp, payload::text, latency_ms, request_id`,
		event.ID, event.RunID, string(event.Type), event.Timestamp, string(payload), event.LatencyMS, event.RequestID)
	return scanRunEvent(row)
}

func (s *PostgresSessionStore) ListRunEventRecords(runID string, limit int) ([]run.RunEvent, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.pool.Query(s.ctx, `SELECT id, run_id, type, timestamp, payload::text, latency_ms, request_id FROM codeflow_run_events WHERE run_id=$1 ORDER BY timestamp ASC LIMIT $2`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []run.RunEvent{}
	for rows.Next() {
		item, err := scanRunEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) CreatePlanRecord(item plan.Plan) (*plan.Plan, error) {
	if strings.TrimSpace(item.ID) == "" {
		item.ID = "plan_" + uuid.NewString()[:8]
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	pref, _ := json.Marshal(item.Preference)
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(s.ctx)
	_, err = tx.Exec(s.ctx, `INSERT INTO codeflow_plans (id, session_id, workspace_id, goal, status, preference, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8)`,
		item.ID, item.SessionID, item.WorkspaceID, item.Goal, string(item.Status), string(pref), item.CreatedAt, item.UpdatedAt)
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
		if err := insertPlanStepPG(ctxOrBackground(s.ctx), tx, step); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(s.ctx); err != nil {
		return nil, err
	}
	return s.GetPlanRecord(item.ID)
}

func (s *PostgresSessionStore) GetPlanRecord(id string) (*plan.Plan, error) {
	row := s.pool.QueryRow(s.ctx, `SELECT id, session_id, workspace_id, goal, status, preference::text, created_at, updated_at FROM codeflow_plans WHERE id=$1`, id)
	item, err := scanPlan(row)
	if err == pgx.ErrNoRows {
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

func (s *PostgresSessionStore) ListPlanRecords(sessionID, workspaceID string, limit int) ([]plan.Plan, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(s.ctx, `SELECT id, session_id, workspace_id, goal, status, preference::text, created_at, updated_at FROM codeflow_plans WHERE ($1='' OR session_id=$1) AND ($2='' OR workspace_id=$2) ORDER BY updated_at DESC LIMIT $3`, sessionID, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.Plan{}
	for rows.Next() {
		item, err := scanPlan(rows)
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

func (s *PostgresSessionStore) UpdatePlanRecord(item plan.Plan) (*plan.Plan, error) {
	item.UpdatedAt = time.Now().UTC()
	pref, _ := json.Marshal(item.Preference)
	row := s.pool.QueryRow(s.ctx, `UPDATE codeflow_plans SET goal=$2, status=$3, preference=$4::jsonb, updated_at=$5 WHERE id=$1 RETURNING id, session_id, workspace_id, goal, status, preference::text, created_at, updated_at`,
		item.ID, item.Goal, string(item.Status), string(pref), item.UpdatedAt)
	return scanPlan(row)
}

func (s *PostgresSessionStore) UpdatePlanStepRecord(step plan.PlanStep) (*plan.PlanStep, error) {
	files, _ := json.Marshal(step.RelatedFiles)
	tools, _ := json.Marshal(step.ToolCalls)
	_, err := s.pool.Exec(s.ctx, `UPDATE codeflow_plan_steps SET title=$2, description=$3, type=$4, status=$5, requires_approval=$6, related_files=$7::jsonb, tool_calls=$8::jsonb, result_summary=$9, error=$10, position=$11 WHERE id=$1`,
		step.ID, step.Title, step.Description, string(step.Type), string(step.Status), step.RequiresApproval, string(files), string(tools), step.ResultSummary, step.Error, step.Position)
	if err != nil {
		return nil, err
	}
	return &step, nil
}

func (s *PostgresSessionStore) CreatePlanEventRecord(event plan.Event) (*plan.Event, error) {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "planevt_" + uuid.NewString()[:8]
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	payload, _ := json.Marshal(event.Payload)
	row := s.pool.QueryRow(s.ctx, `INSERT INTO codeflow_plan_events (id, plan_id, step_id, type, payload, created_at) VALUES ($1,$2,$3,$4,$5::jsonb,$6) RETURNING id, plan_id, step_id, type, payload::text, created_at`,
		event.ID, event.PlanID, event.StepID, event.Type, string(payload), event.CreatedAt)
	return scanPlanEvent(row)
}

func (s *PostgresSessionStore) ListPlanEventRecords(planID string, limit int) ([]plan.Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.pool.Query(s.ctx, `SELECT id, plan_id, step_id, type, payload::text, created_at FROM codeflow_plan_events WHERE plan_id=$1 ORDER BY created_at DESC LIMIT $2`, planID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.Event{}
	for rows.Next() {
		item, err := scanPlanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) CreateCheckpointRecord(item checkpoint.Checkpoint) (*checkpoint.Checkpoint, error) {
	if strings.TrimSpace(item.ID) == "" {
		item.ID = "ckpt_" + uuid.NewString()[:8]
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	changed, _ := json.Marshal(item.ChangedFiles)
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(s.ctx)
	_, err = tx.Exec(s.ctx, `INSERT INTO codeflow_checkpoints (id, workspace_id, session_id, run_id, plan_step_id, created_at, reason, git_head, changed_files, snapshot_path, patch_path, description) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12)`,
		item.ID, item.WorkspaceID, item.SessionID, item.RunID, item.PlanStepID, item.CreatedAt, item.Reason, item.GitHead, string(changed), item.SnapshotPath, item.PatchPath, item.Description)
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
		_, err = tx.Exec(s.ctx, `INSERT INTO codeflow_checkpoint_files (id, checkpoint_id, path, existed, is_binary, size_bytes, sha256, content, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			file.ID, item.ID, file.Path, file.Existed, file.IsBinary, file.SizeBytes, file.SHA256, file.Content, file.CreatedAt)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(s.ctx); err != nil {
		return nil, err
	}
	return s.GetCheckpointRecord(item.ID)
}

func (s *PostgresSessionStore) GetCheckpointRecord(id string) (*checkpoint.Checkpoint, error) {
	row := s.pool.QueryRow(s.ctx, `SELECT id, workspace_id, session_id, run_id, plan_step_id, created_at, reason, git_head, changed_files::text, snapshot_path, patch_path, description FROM codeflow_checkpoints WHERE id=$1`, id)
	item, err := scanCheckpoint(row)
	if err == pgx.ErrNoRows {
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

func (s *PostgresSessionStore) ListCheckpointRecords(sessionID, workspaceID string, limit int) ([]checkpoint.Checkpoint, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(s.ctx, `SELECT id, workspace_id, session_id, run_id, plan_step_id, created_at, reason, git_head, changed_files::text, snapshot_path, patch_path, description FROM codeflow_checkpoints WHERE ($1='' OR session_id=$1) AND ($2='' OR workspace_id=$2) ORDER BY created_at DESC LIMIT $3`, sessionID, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []checkpoint.Checkpoint{}
	for rows.Next() {
		item, err := scanCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) GetSessionSummary(sessionID string) (*SessionSummary, error) {
	row := s.pool.QueryRow(s.ctx, `SELECT session_id, workspace_id, summary, updated_at FROM codeflow_session_summaries WHERE session_id=$1`, sessionID)
	item, err := scanSummary(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *PostgresSessionStore) UpsertSessionSummary(item SessionSummary) (*SessionSummary, error) {
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	row := s.pool.QueryRow(s.ctx, `INSERT INTO codeflow_session_summaries (session_id, workspace_id, summary, updated_at) VALUES ($1,$2,$3,$4) ON CONFLICT (session_id) DO UPDATE SET workspace_id=EXCLUDED.workspace_id, summary=EXCLUDED.summary, updated_at=EXCLUDED.updated_at RETURNING session_id, workspace_id, summary, updated_at`,
		item.SessionID, item.WorkspaceID, item.Summary, item.UpdatedAt)
	return scanSummary(row)
}

func (s *PostgresSessionStore) ClearSessionSummary(sessionID string) error {
	_, err := s.pool.Exec(s.ctx, `DELETE FROM codeflow_session_summaries WHERE session_id=$1`, sessionID)
	return err
}

func (s *PostgresSessionStore) listPlanSteps(planID string) ([]plan.PlanStep, error) {
	rows, err := s.pool.Query(s.ctx, `SELECT id, plan_id, title, description, type, status, requires_approval, related_files::text, tool_calls::text, result_summary, error, position FROM codeflow_plan_steps WHERE plan_id=$1 ORDER BY position ASC`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plan.PlanStep{}
	for rows.Next() {
		item, err := scanPlanStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *PostgresSessionStore) listCheckpointFiles(checkpointID string) ([]checkpoint.FileSnapshot, error) {
	rows, err := s.pool.Query(s.ctx, `SELECT id, checkpoint_id, path, existed, is_binary, size_bytes, sha256, content, created_at FROM codeflow_checkpoint_files WHERE checkpoint_id=$1 ORDER BY created_at ASC`, checkpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []checkpoint.FileSnapshot{}
	for rows.Next() {
		item, err := scanCheckpointFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func insertPlanStepPG(ctx context.Context, tx pgx.Tx, step plan.PlanStep) error {
	files, _ := json.Marshal(step.RelatedFiles)
	tools, _ := json.Marshal(step.ToolCalls)
	_, err := tx.Exec(ctx, `INSERT INTO codeflow_plan_steps (id, plan_id, title, description, type, status, requires_approval, related_files, tool_calls, result_summary, error, position) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12)`,
		step.ID, step.PlanID, step.Title, step.Description, string(step.Type), string(step.Status), step.RequiresApproval, string(files), string(tools), step.ResultSummary, step.Error, step.Position)
	return err
}

func scanRun(row scanner) (*run.AgentRun, error) {
	var item run.AgentRun
	var endedAt *time.Time
	if err := row.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.PlanID, &item.Status, &item.StartedAt, &endedAt, &item.ModelProvider, &item.ModelName, &item.TotalTokens, &item.TotalCost, &item.Error); err != nil {
		return nil, err
	}
	if endedAt != nil {
		item.EndedAt = endedAt.UTC()
	}
	return &item, nil
}

func scanRunEvent(row scanner) (*run.RunEvent, error) {
	var item run.RunEvent
	var payloadJSON string
	if err := row.Scan(&item.ID, &item.RunID, &item.Type, &item.Timestamp, &payloadJSON, &item.LatencyMS, &item.RequestID); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(payloadJSON), &item.Payload)
	return &item, nil
}

func scanPlan(row scanner) (*plan.Plan, error) {
	var item plan.Plan
	var prefJSON string
	if err := row.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.Goal, &item.Status, &prefJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(prefJSON), &item.Preference)
	return &item, nil
}

func scanPlanStep(row scanner) (*plan.PlanStep, error) {
	var item plan.PlanStep
	var filesJSON, toolsJSON string
	if err := row.Scan(&item.ID, &item.PlanID, &item.Title, &item.Description, &item.Type, &item.Status, &item.RequiresApproval, &filesJSON, &toolsJSON, &item.ResultSummary, &item.Error, &item.Position); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(filesJSON), &item.RelatedFiles)
	_ = json.Unmarshal([]byte(toolsJSON), &item.ToolCalls)
	return &item, nil
}

func scanPlanEvent(row scanner) (*plan.Event, error) {
	var item plan.Event
	var payloadJSON string
	if err := row.Scan(&item.ID, &item.PlanID, &item.StepID, &item.Type, &payloadJSON, &item.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(payloadJSON), &item.Payload)
	return &item, nil
}

func scanCheckpoint(row scanner) (*checkpoint.Checkpoint, error) {
	var item checkpoint.Checkpoint
	var changedJSON string
	if err := row.Scan(&item.ID, &item.WorkspaceID, &item.SessionID, &item.RunID, &item.PlanStepID, &item.CreatedAt, &item.Reason, &item.GitHead, &changedJSON, &item.SnapshotPath, &item.PatchPath, &item.Description); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(changedJSON), &item.ChangedFiles)
	return &item, nil
}

func scanCheckpointFile(row scanner) (*checkpoint.FileSnapshot, error) {
	var item checkpoint.FileSnapshot
	if err := row.Scan(&item.ID, &item.CheckpointID, &item.Path, &item.Existed, &item.IsBinary, &item.SizeBytes, &item.SHA256, &item.Content, &item.CreatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanSummary(row scanner) (*SessionSummary, error) {
	var item SessionSummary
	if err := row.Scan(&item.SessionID, &item.WorkspaceID, &item.Summary, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func zeroTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
