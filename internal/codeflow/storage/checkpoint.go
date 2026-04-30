package storage

import "github.com/viko0313/CodeFlow/internal/codeflow/checkpoint"

type CheckpointStore interface {
	CreateCheckpointRecord(item checkpoint.Checkpoint) (*checkpoint.Checkpoint, error)
	GetCheckpointRecord(id string) (*checkpoint.Checkpoint, error)
	ListCheckpointRecords(sessionID, workspaceID string, limit int) ([]checkpoint.Checkpoint, error)
}
