package plan

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	store Store
}

type Store interface {
	CreatePlanRecord(item Plan) (*Plan, error)
	GetPlanRecord(id string) (*Plan, error)
	ListPlanRecords(sessionID, workspaceID string, limit int) ([]Plan, error)
	UpdatePlanRecord(item Plan) (*Plan, error)
	UpdatePlanStepRecord(step PlanStep) (*PlanStep, error)
	CreatePlanEventRecord(event Event) (*Event, error)
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(item Plan) (*Plan, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("plan store is not configured")
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = "plan_" + uuid.NewString()[:8]
	}
	if item.Status == "" {
		item.Status = StatusPlanning
	}
	for i := range item.Steps {
		if strings.TrimSpace(item.Steps[i].ID) == "" {
			item.Steps[i].ID = "step_" + uuid.NewString()[:8]
		}
		if item.Steps[i].Status == "" {
			item.Steps[i].Status = StepPending
		}
		item.Steps[i].Position = i
	}
	created, err := s.store.CreatePlanRecord(item)
	if err != nil {
		return nil, err
	}
	_, _ = s.store.CreatePlanEventRecord(Event{PlanID: created.ID, Type: "plan.created", CreatedAt: time.Now().UTC(), Payload: map[string]any{"status": created.Status}})
	return created, nil
}

func (s *Service) Get(id string) (*Plan, error) { return s.store.GetPlanRecord(id) }
func (s *Service) List(sessionID, workspaceID string, limit int) ([]Plan, error) {
	return s.store.ListPlanRecords(sessionID, workspaceID, limit)
}

func (s *Service) Approve(id string) (*Plan, error) {
	return s.setStatus(id, StatusActing, "plan.approved")
}
func (s *Service) Pause(id string) (*Plan, error) {
	return s.setStatus(id, StatusPaused, "plan.paused")
}
func (s *Service) Resume(id string) (*Plan, error) {
	return s.setStatus(id, StatusActing, "plan.resumed")
}

func (s *Service) UpdateStep(step PlanStep) (*PlanStep, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("plan store is not configured")
	}
	updated, err := s.store.UpdatePlanStepRecord(step)
	if err != nil {
		return nil, err
	}
	_, _ = s.store.CreatePlanEventRecord(Event{PlanID: step.PlanID, StepID: step.ID, Type: "plan.step.updated", CreatedAt: time.Now().UTC(), Payload: map[string]any{"status": step.Status, "title": step.Title}})
	return updated, nil
}

func (s *Service) setStatus(id string, status Status, eventType string) (*Plan, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("plan store is not configured")
	}
	item, err := s.store.GetPlanRecord(id)
	if err != nil || item == nil {
		return item, err
	}
	item.Status = status
	updated, err := s.store.UpdatePlanRecord(*item)
	if err != nil {
		return nil, err
	}
	_, _ = s.store.CreatePlanEventRecord(Event{PlanID: id, Type: eventType, CreatedAt: time.Now().UTC(), Payload: map[string]any{"status": status}})
	return updated, nil
}
