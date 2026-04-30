package storage

import (
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
)

type RunStore interface {
	CreateRunRecord(record run.AgentRun) (*run.AgentRun, error)
	UpdateRunRecord(record run.AgentRun) (*run.AgentRun, error)
	ListRunRecords(sessionID, workspaceID string, limit int) ([]run.AgentRun, error)
	GetRunRecord(id string) (*run.AgentRun, error)
	CreateRunEventRecord(event run.RunEvent) (*run.RunEvent, error)
	ListRunEventRecords(runID string, limit int) ([]run.RunEvent, error)
}
