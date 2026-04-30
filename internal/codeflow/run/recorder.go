package run

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Recorder struct {
	store Store
}

type Store interface {
	CreateRunRecord(record AgentRun) (*AgentRun, error)
	UpdateRunRecord(record AgentRun) (*AgentRun, error)
	CreateRunEventRecord(event RunEvent) (*RunEvent, error)
}

func NewRecorder(store Store) *Recorder {
	return &Recorder{store: store}
}

func (r *Recorder) Enabled() bool {
	return r != nil && r.store != nil
}

func (r *Recorder) Start(ctx context.Context, record AgentRun) (*AgentRun, error) {
	_ = ctx
	if !r.Enabled() {
		return &record, nil
	}
	record.ID = ensureID(record.ID, "run_")
	record.Status = StatusRunning
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now().UTC()
	}
	return r.store.CreateRunRecord(record)
}

func (r *Recorder) Finish(ctx context.Context, record AgentRun, status Status, errText string) (*AgentRun, error) {
	_ = ctx
	if !r.Enabled() {
		record.Status = status
		record.Error = errText
		record.EndedAt = time.Now().UTC()
		return &record, nil
	}
	record.Status = status
	record.Error = errText
	record.EndedAt = time.Now().UTC()
	return r.store.UpdateRunRecord(record)
}

func (r *Recorder) Event(ctx context.Context, event RunEvent) error {
	_ = ctx
	if !r.Enabled() {
		return nil
	}
	event.ID = ensureID(event.ID, "runevt_")
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	_, err := r.store.CreateRunEventRecord(event)
	return err
}

func ensureID(id, prefix string) string {
	if id != "" {
		return id
	}
	return prefix + uuid.NewString()[:8]
}
