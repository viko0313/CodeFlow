package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type TodoStore struct {
	mu    sync.Mutex
	items []TodoItem
}

func NewTodoStore() *TodoStore {
	return &TodoStore{}
}

func (s *TodoStore) Upsert(id, content, status string) TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	status = normalizeTodoStatus(status)
	content = strings.TrimSpace(content)
	if strings.TrimSpace(id) == "" {
		id = "todo_" + uuid.NewString()[:8]
	}
	for i := range s.items {
		if s.items[i].ID == id {
			if content != "" {
				s.items[i].Content = content
			}
			s.items[i].Status = status
			return s.items[i]
		}
	}
	item := TodoItem{ID: id, Content: content, Status: status}
	s.items = append(s.items, item)
	return item
}

func (s *TodoStore) List() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TodoItem, len(s.items))
	copy(out, s.items)
	return out
}

func (s *TodoStore) Snapshot() string {
	items := s.List()
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Active todo state]\n")
	for _, item := range items {
		if item.Status == "completed" || item.Status == "cancelled" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item.ID)
		b.WriteString(" [")
		b.WriteString(item.Status)
		b.WriteString("] ")
		b.WriteString(item.Content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func TodoToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "todo",
		Description: "Create, update, or list the current session task list.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema: &schema.ToolInfo{Name: "todo", Desc: "Create, update, or list the current session task list.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":  {Type: schema.String, Desc: "Action to perform.", Enum: []string{"upsert", "list"}, Required: true},
			"id":      {Type: schema.String, Desc: "Existing todo id when updating."},
			"content": {Type: schema.String, Desc: "Task content for upsert."},
			"status":  {Type: schema.String, Desc: "Task status.", Enum: []string{"pending", "in_progress", "completed", "cancelled"}},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			_ = ctx
			if runtime.Todos == nil {
				return ToolResult{}, fmt.Errorf("todo store is not configured")
			}
			var input struct {
				Action  string `json:"action"`
				ID      string `json:"id"`
				Content string `json:"content"`
				Status  string `json:"status"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			switch strings.ToLower(strings.TrimSpace(input.Action)) {
			case "list":
				data, _ := json.Marshal(runtime.Todos.List())
				return ToolResult{Content: string(data)}, nil
			case "upsert", "":
				item := runtime.Todos.Upsert(input.ID, input.Content, input.Status)
				data, _ := json.Marshal(item)
				return ToolResult{Content: string(data)}, nil
			default:
				return ToolResult{}, fmt.Errorf("unsupported todo action: %s", input.Action)
			}
		},
	}
}

func normalizeTodoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_progress", "completed", "cancelled":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "pending"
	}
}
