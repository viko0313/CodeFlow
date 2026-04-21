package heartbeat

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/viko0313/CodeFlow/internal/bus"
	"github.com/viko0313/CodeFlow/internal/tools"
)

func StartPacemaker(ctx context.Context, tasksFile string, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggerDue(tasksFile)
			}
		}
	}()
}

func triggerDue(tasksFile string) {
	data, err := os.ReadFile(tasksFile)
	if err != nil || len(data) == 0 {
		return
	}
	var tasks []tools.ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return
	}
	now := time.Now()
	pending := make([]tools.ScheduledTask, 0, len(tasks))
	for _, task := range tasks {
		target, err := time.ParseInLocation("2006-01-02 15:04:05", task.TargetTime, time.Local)
		if err != nil {
			continue
		}
		if now.Before(target) {
			pending = append(pending, task)
			continue
		}
		bus.EmitTask(bus.TaskMessage{
			Type:      "scheduled_task",
			Content:   task.Description,
			Priority:  1,
			Timestamp: now.Unix(),
		})
		if next, ok := nextRepeat(task, target); ok {
			task.TargetTime = next.Format("2006-01-02 15:04:05")
			pending = append(pending, task)
		}
	}
	out, err := json.MarshalIndent(pending, "", "  ")
	if err == nil {
		_ = os.WriteFile(tasksFile, out, 0600)
	}
}

func nextRepeat(task tools.ScheduledTask, target time.Time) (time.Time, bool) {
	if task.Repeat == "" {
		return time.Time{}, false
	}
	if task.RepeatCount != nil {
		if *task.RepeatCount <= 1 {
			return time.Time{}, false
		}
		*task.RepeatCount--
	}
	switch task.Repeat {
	case "hourly":
		return target.Add(time.Hour), true
	case "daily":
		return target.AddDate(0, 0, 1), true
	case "weekly":
		return target.AddDate(0, 0, 7), true
	default:
		return time.Time{}, false
	}
}
