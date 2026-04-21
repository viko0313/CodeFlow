package approval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
)

var (
	ErrApprovalNotFound       = errors.New("approval not found")
	ErrApprovalAlreadyDecided = errors.New("approval already decided")
	ErrRejectReasonRequired   = errors.New("reject reason is required")
)

type Service struct {
	store storage.ApprovalStore
}

func NewService(store storage.ApprovalStore) *Service {
	return &Service{store: store}
}

func (s *Service) Enabled() bool {
	return s != nil && s.store != nil
}

func (s *Service) Store() storage.ApprovalStore {
	if s == nil {
		return nil
	}
	return s.store
}

func (s *Service) List(status string, limit int) ([]storage.ApprovalRecord, error) {
	if !s.Enabled() {
		return []storage.ApprovalRecord{}, nil
	}
	return s.store.ListApprovals(storage.ListApprovalsOptions{Status: status, Limit: limit})
}

func (s *Service) Get(id string) (*storage.ApprovalRecord, error) {
	if !s.Enabled() {
		return nil, ErrApprovalNotFound
	}
	record, err := s.store.GetApproval(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrApprovalNotFound
	}
	return record, nil
}

func (s *Service) Decide(id string, allowed bool, reason string) (*storage.ApprovalRecord, error) {
	if !s.Enabled() {
		return nil, ErrApprovalNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrApprovalNotFound
	}
	reason = strings.TrimSpace(reason)
	if !allowed && reason == "" {
		return nil, ErrRejectReasonRequired
	}
	record, err := s.store.DecideApproval(id, allowed, reason)
	if err != nil {
		return nil, err
	}
	if record == nil {
		existing, getErr := s.store.GetApproval(id)
		if getErr != nil {
			return nil, getErr
		}
		if existing == nil {
			return nil, ErrApprovalNotFound
		}
		return nil, ErrApprovalAlreadyDecided
	}
	return record, nil
}

func (s *Service) WaitForDecision(ctx context.Context, approvalID string) (bool, string, error) {
	if !s.Enabled() {
		return false, "", ErrApprovalNotFound
	}
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return false, "", ErrApprovalNotFound
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		record, err := s.store.GetApproval(approvalID)
		if err != nil {
			return false, "", err
		}
		if record == nil {
			return false, "", ErrApprovalNotFound
		}
		switch record.Status {
		case storage.ApprovalStatusApproved:
			return true, record.DecisionReason, nil
		case storage.ApprovalStatusRejected:
			reason := strings.TrimSpace(record.DecisionReason)
			if reason == "" {
				reason = "rejected"
			}
			return false, reason, nil
		}
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-ticker.C:
		}
	}
}
