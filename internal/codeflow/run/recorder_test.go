package run

import (
	"context"
	"testing"
)

type fakeStore struct {
	run    AgentRun
	events []RunEvent
}

func (s *fakeStore) CreateRunRecord(record AgentRun) (*AgentRun, error) {
	s.run = record
	return &record, nil
}

func (s *fakeStore) UpdateRunRecord(record AgentRun) (*AgentRun, error) {
	s.run = record
	return &record, nil
}

func (s *fakeStore) CreateRunEventRecord(event RunEvent) (*RunEvent, error) {
	s.events = append(s.events, event)
	return &event, nil
}

func TestRecorderStartEventFinish(t *testing.T) {
	store := &fakeStore{}
	recorder := NewRecorder(store)
	record, err := recorder.Start(context.Background(), AgentRun{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.Status != StatusRunning {
		t.Fatalf("unexpected run record: %+v", record)
	}
	if err := recorder.Event(context.Background(), RunEvent{RunID: record.ID, Type: EventUserInput}); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.Finish(context.Background(), *record, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 || store.run.Status != StatusCompleted {
		t.Fatalf("unexpected recorder state: %+v %+v", store.run, store.events)
	}
}
