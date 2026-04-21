package heartbeat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viko0313/CodeFlow/internal/bus"
	"github.com/viko0313/CodeFlow/internal/tools"
)

func TestTriggerDueTasksAndRepeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	count := 2
	due := time.Now().Add(-time.Minute).Format("2006-01-02 15:04:05")
	tasks := []tools.ScheduledTask{{
		ID:          "task1",
		TargetTime:  due,
		Description: "wake up",
		Repeat:      "daily",
		RepeatCount: &count,
	}}
	data, _ := json.Marshal(tasks)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	triggerDue(path)

	select {
	case msg := <-bus.GetTaskQueue():
		if !strings.Contains(msg.Content, "wake up") {
			t.Fatalf("unexpected message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected task event")
	}

	updatedRaw, _ := os.ReadFile(path)
	var updated []tools.ScheduledTask
	if err := json.Unmarshal(updatedRaw, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].RepeatCount == nil || *updated[0].RepeatCount != 1 {
		t.Fatalf("repeat task was not rescheduled correctly: %+v", updated)
	}
}
