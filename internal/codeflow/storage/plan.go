package storage

import "github.com/viko0313/CodeFlow/internal/codeflow/plan"

type PlanStore interface {
	CreatePlanRecord(item plan.Plan) (*plan.Plan, error)
	GetPlanRecord(id string) (*plan.Plan, error)
	ListPlanRecords(sessionID, workspaceID string, limit int) ([]plan.Plan, error)
	UpdatePlanRecord(item plan.Plan) (*plan.Plan, error)
	UpdatePlanStepRecord(step plan.PlanStep) (*plan.PlanStep, error)
	CreatePlanEventRecord(event plan.Event) (*plan.Event, error)
	ListPlanEventRecords(planID string, limit int) ([]plan.Event, error)
}
