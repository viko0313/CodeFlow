package plan

import "testing"

type fakePlanStore struct {
	plan   *Plan
	events []Event
}

func (s *fakePlanStore) CreatePlanRecord(item Plan) (*Plan, error) { s.plan = &item; return &item, nil }
func (s *fakePlanStore) GetPlanRecord(id string) (*Plan, error)    { return s.plan, nil }
func (s *fakePlanStore) ListPlanRecords(sessionID, workspaceID string, limit int) ([]Plan, error) {
	if s.plan == nil {
		return []Plan{}, nil
	}
	return []Plan{*s.plan}, nil
}
func (s *fakePlanStore) UpdatePlanRecord(item Plan) (*Plan, error) { s.plan = &item; return &item, nil }
func (s *fakePlanStore) UpdatePlanStepRecord(step PlanStep) (*PlanStep, error) {
	for i := range s.plan.Steps {
		if s.plan.Steps[i].ID == step.ID {
			s.plan.Steps[i] = step
		}
	}
	return &step, nil
}
func (s *fakePlanStore) CreatePlanEventRecord(event Event) (*Event, error) {
	s.events = append(s.events, event)
	return &event, nil
}

func TestPlanServiceCreateApprovePauseResume(t *testing.T) {
	store := &fakePlanStore{}
	svc := NewService(store)
	item, err := svc.Create(Plan{SessionID: "s1", Goal: "ship it", Steps: []PlanStep{{Title: "step1", Type: StepRead}}})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.Status != StatusPlanning {
		t.Fatalf("unexpected created plan: %+v", item)
	}
	item, err = svc.Approve(item.ID)
	if err != nil || item.Status != StatusActing {
		t.Fatalf("approve failed: %+v %v", item, err)
	}
	item, err = svc.Pause(item.ID)
	if err != nil || item.Status != StatusPaused {
		t.Fatalf("pause failed: %+v %v", item, err)
	}
	item, err = svc.Resume(item.ID)
	if err != nil || item.Status != StatusActing {
		t.Fatalf("resume failed: %+v %v", item, err)
	}
}
