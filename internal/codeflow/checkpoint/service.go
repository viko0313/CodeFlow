package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/viko0313/CodeFlow/internal/codeflow/observability"
	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
)

const DefaultMaxSnapshotBytes int64 = 1024 * 1024

type Service struct {
	store    Store
	recorder *run.Recorder
	maxBytes int64
}

type Store interface {
	CreateCheckpointRecord(item Checkpoint) (*Checkpoint, error)
	GetCheckpointRecord(id string) (*Checkpoint, error)
	ListCheckpointRecords(sessionID, workspaceID string, limit int) ([]Checkpoint, error)
}

func NewService(store Store, recorder *run.Recorder) *Service {
	return &Service{store: store, recorder: recorder, maxBytes: DefaultMaxSnapshotBytes}
}

func (s *Service) CreateForWrite(ctx context.Context, workspaceID, sessionID, runID, planStepID, projectRoot string, paths []string, reason string) (*Checkpoint, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	item := Checkpoint{
		ID:          "ckpt_" + uuid.NewString()[:8],
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		RunID:       runID,
		PlanStepID:  planStepID,
		CreatedAt:   time.Now().UTC(),
		Reason:      reason,
		GitHead:     gitHead(projectRoot),
	}
	for _, relPath := range paths {
		target, err := permission.ValidateProjectPath(projectRoot, relPath)
		if err != nil {
			return nil, err
		}
		snap, err := snapshotFile(target, relPath, s.maxBytes)
		if err != nil {
			return nil, err
		}
		item.ChangedFiles = append(item.ChangedFiles, relPath)
		item.Files = append(item.Files, snap)
	}
	created, err := s.store.CreateCheckpointRecord(item)
	if err != nil {
		return nil, err
	}
	if s.recorder != nil && runID != "" {
		_ = s.recorder.Event(ctx, run.RunEvent{RunID: runID, Type: run.EventCheckpointCreated, Timestamp: time.Now().UTC(), RequestID: observability.RequestIDFromContext(ctx), Payload: map[string]any{"checkpoint_id": created.ID, "files": created.ChangedFiles}})
	}
	return created, nil
}

func (s *Service) List(sessionID, workspaceID string, limit int) ([]Checkpoint, error) {
	if s == nil || s.store == nil {
		return []Checkpoint{}, nil
	}
	return s.store.ListCheckpointRecords(sessionID, workspaceID, limit)
}

func (s *Service) Get(id string) (*Checkpoint, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.GetCheckpointRecord(id)
}

func (s *Service) Rewind(ctx context.Context, projectRoot string, item *Checkpoint) error {
	if item == nil {
		return fmt.Errorf("checkpoint is required")
	}
	for _, file := range item.Files {
		target, err := permission.ValidateProjectPath(projectRoot, file.Path)
		if err != nil {
			return err
		}
		if !file.Existed {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if file.IsBinary && file.Content == "" {
			return fmt.Errorf("binary rewind is not supported for %s", file.Path)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func snapshotFile(target, relPath string, maxBytes int64) (FileSnapshot, error) {
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return FileSnapshot{Path: relPath, Existed: false, CreatedAt: time.Now().UTC()}, nil
		}
		return FileSnapshot{}, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return FileSnapshot{}, err
	}
	sum := sha256.Sum256(data)
	snap := FileSnapshot{
		Path:      relPath,
		Existed:   true,
		SizeBytes: info.Size(),
		SHA256:    hex.EncodeToString(sum[:]),
		CreatedAt: time.Now().UTC(),
	}
	if info.Size() > maxBytes || !isText(data) {
		snap.IsBinary = !isText(data)
		return snap, nil
	}
	snap.Content = string(data)
	return snap, nil
}

func isText(data []byte) bool {
	return strings.IndexByte(string(data), 0) < 0
}

func gitHead(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
