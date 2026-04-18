package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

var tasksMu sync.Mutex

type ScheduledTask struct {
	ID          string `json:"id"`
	TargetTime  string `json:"target_time"`
	Description string `json:"description"`
	Repeat      string `json:"repeat,omitempty"`
	RepeatCount *int   `json:"repeat_count,omitempty"`
}

type TaskStore struct {
	path string
}

func NewTaskStore(path string) *TaskStore {
	return &TaskStore{path: path}
}

func (s *TaskStore) load() ([]ScheduledTask, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var tasks []ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *TaskStore) save(tasks []ScheduledTask) error {
	if err := os.MkdirAll(filepathDir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

type ScheduleTaskTool struct{ store *TaskStore }
type ListScheduledTasksTool struct{ store *TaskStore }
type DeleteScheduledTaskTool struct{ store *TaskStore }
type ModifyScheduledTaskTool struct{ store *TaskStore }

func NewScheduleTaskTool(path string) *ScheduleTaskTool {
	return &ScheduleTaskTool{store: NewTaskStore(path)}
}
func NewListScheduledTasksTool(path string) *ListScheduledTasksTool {
	return &ListScheduledTasksTool{store: NewTaskStore(path)}
}
func NewDeleteScheduledTaskTool(path string) *DeleteScheduledTaskTool {
	return &DeleteScheduledTaskTool{store: NewTaskStore(path)}
}
func NewModifyScheduledTaskTool(path string) *ModifyScheduledTaskTool {
	return &ModifyScheduledTaskTool{store: NewTaskStore(path)}
}

func (t *ScheduleTaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "schedule_task", Desc: "Schedule a future reminder task. target_time must be YYYY-MM-DD HH:MM:SS.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"target_time":  {Type: schema.String, Desc: "YYYY-MM-DD HH:MM:SS", Required: true},
		"description":  {Type: schema.String, Desc: "Reminder content", Required: true},
		"repeat":       {Type: schema.String, Desc: "Optional: hourly, daily, weekly"},
		"repeat_count": {Type: schema.Integer, Desc: "Optional total trigger count"},
	})}, nil
}

func (t *ScheduleTaskTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		TargetTime  string `json:"target_time"`
		Description string `json:"description"`
		Repeat      string `json:"repeat"`
		RepeatCount *int   `json:"repeat_count"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", err
	}
	if _, err := time.ParseInLocation("2006-01-02 15:04:05", args.TargetTime, time.Local); err != nil {
		return "", fmt.Errorf("invalid target_time format, expected YYYY-MM-DD HH:MM:SS")
	}
	if args.Repeat != "" && args.Repeat != "hourly" && args.Repeat != "daily" && args.Repeat != "weekly" {
		return "", fmt.Errorf("repeat must be hourly, daily, weekly, or empty")
	}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	tasks, err := t.store.load()
	if err != nil {
		return "", err
	}
	task := ScheduledTask{ID: uuid.NewString()[:8], TargetTime: args.TargetTime, Description: args.Description, Repeat: args.Repeat, RepeatCount: args.RepeatCount}
	tasks = append(tasks, task)
	if err := t.store.save(tasks); err != nil {
		return "", err
	}
	return fmt.Sprintf("Task scheduled: [%s] %s | %s", task.ID, task.TargetTime, task.Description), nil
}

func (t *ListScheduledTasksTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "list_scheduled_tasks", Desc: "List all scheduled reminder tasks."}, nil
}

func (t *ListScheduledTasksTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	tasks, err := t.store.load()
	if err != nil {
		return "", err
	}
	return FormatTasks(tasks), nil
}

func (t *DeleteScheduledTaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "delete_scheduled_task", Desc: "Delete a scheduled task by exact task_id.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"task_id": {Type: schema.String, Desc: "Exact task ID", Required: true},
	})}, nil
}

func (t *DeleteScheduledTaskTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", err
	}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	tasks, err := t.store.load()
	if err != nil {
		return "", err
	}
	next := tasks[:0]
	found := false
	for _, task := range tasks {
		if task.ID == args.TaskID {
			found = true
			continue
		}
		next = append(next, task)
	}
	if !found {
		return "", fmt.Errorf("task not found: %s", args.TaskID)
	}
	if err := t.store.save(next); err != nil {
		return "", err
	}
	return fmt.Sprintf("Task [%s] deleted.", args.TaskID), nil
}

func (t *ModifyScheduledTaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "modify_scheduled_task", Desc: "Modify a scheduled task by exact task_id.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"task_id":         {Type: schema.String, Desc: "Exact task ID", Required: true},
		"new_time":        {Type: schema.String, Desc: "Optional new YYYY-MM-DD HH:MM:SS"},
		"new_description": {Type: schema.String, Desc: "Optional new description"},
	})}, nil
}

func (t *ModifyScheduledTaskTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		TaskID         string `json:"task_id"`
		NewTime        string `json:"new_time"`
		NewDescription string `json:"new_description"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", err
	}
	if args.NewTime != "" {
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", args.NewTime, time.Local); err != nil {
			return "", fmt.Errorf("invalid new_time format")
		}
	}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	tasks, err := t.store.load()
	if err != nil {
		return "", err
	}
	for i := range tasks {
		if tasks[i].ID == args.TaskID {
			if args.NewTime != "" {
				tasks[i].TargetTime = args.NewTime
			}
			if args.NewDescription != "" {
				tasks[i].Description = args.NewDescription
			}
			if err := t.store.save(tasks); err != nil {
				return "", err
			}
			return fmt.Sprintf("Task [%s] updated.", args.TaskID), nil
		}
	}
	return "", fmt.Errorf("task not found: %s", args.TaskID)
}

func FormatTasks(tasks []ScheduledTask) string {
	if len(tasks) == 0 {
		return "No scheduled tasks."
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].TargetTime < tasks[j].TargetTime })
	out := "Scheduled tasks:\n"
	for _, task := range tasks {
		repeat := ""
		if task.Repeat != "" {
			repeat = fmt.Sprintf(" | repeat=%s", task.Repeat)
		}
		out += fmt.Sprintf("- [ID: %s] %s | %s%s\n", task.ID, task.TargetTime, task.Description, repeat)
	}
	return out
}

func filepathDir(path string) string {
	idx := len(path) - 1
	for idx >= 0 && path[idx] != '/' && path[idx] != '\\' {
		idx--
	}
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}
