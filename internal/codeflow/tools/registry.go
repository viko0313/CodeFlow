package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
)

const DefaultToolset = "codeflow"

type ToolHandler func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error)

type ToolSpec struct {
	Name        string
	Description string
	Toolset     string
	Risk        string
	Schema      *schema.ToolInfo
	Handler     ToolHandler
}

type ToolResult struct {
	Content    string
	Meta       map[string]any
	OK         bool
	ErrorType  string
	Retryable  bool
	DurationMS int64
}

type ToolRuntime struct {
	ProjectRoot string
	SessionID   string
	RequestID   string
	Executor    *Executor
	Todos       *TodoStore
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolSpec
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]ToolSpec{}}
}

func (r *ToolRegistry) Register(spec ToolSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("tool name is required")
	}
	if spec.Handler == nil {
		return fmt.Errorf("tool handler is required: %s", spec.Name)
	}
	if spec.Schema == nil {
		spec.Schema = &schema.ToolInfo{Name: spec.Name, Desc: spec.Description}
	}
	if strings.TrimSpace(spec.Schema.Name) == "" {
		spec.Schema.Name = spec.Name
	}
	if strings.TrimSpace(spec.Schema.Desc) == "" {
		spec.Schema.Desc = spec.Description
	}
	if strings.TrimSpace(spec.Toolset) == "" {
		spec.Toolset = DefaultToolset
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[spec.Name]; exists {
		return fmt.Errorf("tool already registered: %s", spec.Name)
	}
	r.tools[spec.Name] = spec
	return nil
}

func (r *ToolRegistry) Definitions(ctx context.Context, toolset string) ([]*schema.ToolInfo, error) {
	_ = ctx
	toolset = normalizeToolset(toolset)
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name, spec := range r.tools {
		if normalizeToolset(spec.Toolset) == toolset {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]*schema.ToolInfo, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name].Schema)
	}
	return out, nil
}

func (r *ToolRegistry) Dispatch(ctx context.Context, call schema.ToolCall, runtime ToolRuntime) (ToolResult, error) {
	start := time.Now()
	name := strings.TrimSpace(call.Function.Name)
	r.mu.RLock()
	spec, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ToolResult{Content: fmt.Sprintf(`{"error":"unknown tool","tool":%q}`, name), OK: false, ErrorType: "unknown_tool", DurationMS: elapsedMS(start)}, nil
	}
	result, err := spec.Handler(ctx, json.RawMessage(call.Function.Arguments), runtime)
	if err != nil {
		errorType := classifyToolError(err)
		return ToolResult{Content: fmt.Sprintf(`{"error":%q,"tool":%q,"error_type":%q}`, err.Error(), name, errorType), OK: false, ErrorType: errorType, Retryable: isRetryableToolError(errorType), DurationMS: elapsedMS(start)}, nil
	}
	result.OK = true
	result.DurationMS = elapsedMS(start)
	if strings.TrimSpace(result.Content) == "" && result.Meta != nil {
		data, marshalErr := json.Marshal(result.Meta)
		if marshalErr == nil {
			result.Content = string(data)
		}
	}
	return result, nil
}

func WarningToolResult(content, errorType string) ToolResult {
	return ToolResult{Content: content, OK: false, ErrorType: errorType, Retryable: false}
}

func DefaultRegistry() *ToolRegistry {
	registry := NewToolRegistry()
	_ = registry.Register(TodoToolSpec())
	_ = registry.Register(RunShellToolSpec())
	_ = registry.Register(WriteFileToolSpec())
	RegisterCodingTools(registry)
	return registry
}

func normalizeToolset(toolset string) string {
	toolset = strings.TrimSpace(toolset)
	if toolset == "" {
		return DefaultToolset
	}
	return toolset
}

func seconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}

func elapsedMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func classifyToolError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "no such file"):
		return "not_found"
	case strings.Contains(msg, "path escapes"), strings.Contains(msg, "absolute paths"), strings.Contains(msg, "security protocol"):
		return "permission"
	case strings.Contains(msg, "denied"), strings.Contains(msg, "approval"):
		return "approval_denied"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "json"), strings.Contains(msg, "required"), strings.Contains(msg, "invalid"):
		return "invalid_args"
	case strings.Contains(msg, "exit status"), strings.Contains(msg, "command"):
		return "command_failed"
	default:
		return "tool_error"
	}
}

func isRetryableToolError(errorType string) bool {
	return errorType == "timeout" || errorType == "command_failed" || errorType == "tool_error"
}

func RunShellToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "run_shell",
		Description: "Run a shell command in the project root after CodeFlow permission approval.",
		Toolset:     DefaultToolset,
		Risk:        "high",
		Schema: &schema.ToolInfo{Name: "run_shell", Desc: "Run a shell command in the project root after CodeFlow permission approval.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command":         {Type: schema.String, Desc: "Shell command to execute.", Required: true},
			"timeout_seconds": {Type: schema.Integer, Desc: "Optional timeout in seconds. Defaults to 60."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			if runtime.Executor == nil {
				return ToolResult{}, fmt.Errorf("executor is not configured")
			}
			var input struct {
				Command        string `json:"command"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			timeout := input.TimeoutSeconds
			if timeout <= 0 {
				timeout = 60
			}
			result, err := runtime.Executor.Execute(ctx, Operation{
				Kind:        permission.OperationShell,
				ProjectRoot: runtime.ProjectRoot,
				Command:     input.Command,
				Timeout:     seconds(timeout),
				RequestID:   runtime.RequestID,
			}, runtime.SessionID)
			if err != nil {
				return ToolResult{}, err
			}
			return ToolResult{Content: result.Output, Meta: map[string]any{"approval_id": result.ApprovalID, "confirmed": result.Confirmed}}, nil
		},
	}
}

func WriteFileToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "write_file",
		Description: "Write or append a project-root file after CodeFlow permission approval and diff preview.",
		Toolset:     DefaultToolset,
		Risk:        "high",
		Schema: &schema.ToolInfo{Name: "write_file", Desc: "Write or append a project-root file after CodeFlow permission approval and diff preview.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Desc: "Project-relative file path.", Required: true},
			"content": {Type: schema.String, Desc: "Complete content to write or append.", Required: true},
			"append":  {Type: schema.Boolean, Desc: "Append instead of replacing the file."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			if runtime.Executor == nil {
				return ToolResult{}, fmt.Errorf("executor is not configured")
			}
			var input struct {
				Path    string `json:"path"`
				Content string `json:"content"`
				Append  bool   `json:"append"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			result, err := runtime.Executor.Execute(ctx, Operation{
				Kind:        permission.OperationWriteFile,
				ProjectRoot: runtime.ProjectRoot,
				Path:        input.Path,
				Content:     input.Content,
				Append:      input.Append,
				RequestID:   runtime.RequestID,
			}, runtime.SessionID)
			if err != nil {
				return ToolResult{}, err
			}
			return ToolResult{Content: result.Output, Meta: map[string]any{"approval_id": result.ApprovalID, "confirmed": result.Confirmed}}, nil
		},
	}
}
