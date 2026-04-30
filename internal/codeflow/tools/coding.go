package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
)

const maxToolOutput = 12000

func RegisterCodingTools(registry *ToolRegistry) {
	_ = registry.Register(ListFilesToolSpec())
	_ = registry.Register(ReadFileToolSpec())
	_ = registry.Register(SearchCodeToolSpec())
	_ = registry.Register(ApplyPatchToolSpec())
	_ = registry.Register(GlobToolSpec())
	_ = registry.Register(GrepToolSpec())
	_ = registry.Register(EditFileToolSpec())
	_ = registry.Register(ExecuteToolSpec())
	_ = registry.Register(GitStatusToolSpec())
	_ = registry.Register(GitDiffToolSpec())
	_ = registry.Register(RunCheckToolSpec())
}

func ListFilesToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "list_files",
		Description: "List project files under an optional project-relative directory.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema: &schema.ToolInfo{Name: "list_files", Desc: "List project files under an optional project-relative directory.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":      {Type: schema.String, Desc: "Optional project-relative directory. Defaults to project root."},
			"max_files": {Type: schema.Integer, Desc: "Maximum files to return. Defaults to 200."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			_ = ctx
			var input struct {
				Path     string `json:"path"`
				MaxFiles int    `json:"max_files"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			root, err := cleanRoot(runtime.ProjectRoot)
			if err != nil {
				return ToolResult{}, err
			}
			target := root
			if strings.TrimSpace(input.Path) != "" {
				target, err = permission.ValidateProjectPath(root, input.Path)
				if err != nil {
					return ToolResult{}, err
				}
			}
			limit := clamp(input.MaxFiles, 200, 1000)
			files := make([]string, 0, limit)
			err = filepath.WalkDir(target, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if d.IsDir() && shouldSkipDir(d.Name()) && path != target {
					return filepath.SkipDir
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return nil
				}
				files = append(files, filepath.ToSlash(rel))
				if len(files) >= limit {
					return errStopWalk
				}
				return nil
			})
			if err != nil && err != errStopWalk {
				return ToolResult{}, err
			}
			sort.Strings(files)
			data, _ := json.Marshal(map[string]any{"files": files, "truncated": len(files) >= limit})
			return ToolResult{Content: string(data)}, nil
		},
	}
}

func ReadFileToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from the project root.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema: &schema.ToolInfo{Name: "read_file", Desc: "Read a UTF-8 text file from the project root.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Project-relative file path.", Required: true},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			_ = ctx
			var input struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			target, err := permission.ValidateProjectPath(runtime.ProjectRoot, input.Path)
			if err != nil {
				return ToolResult{}, err
			}
			data, err := os.ReadFile(target)
			if err != nil {
				return ToolResult{}, err
			}
			if !utf8.Valid(data) {
				return ToolResult{}, fmt.Errorf("file is not valid UTF-8: %s", input.Path)
			}
			content := truncate(string(data), maxToolOutput)
			out, _ := json.Marshal(map[string]any{"path": input.Path, "content": content, "truncated": len(data) > len(content)})
			return ToolResult{Content: string(out)}, nil
		},
	}
}

func SearchCodeToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "search_code",
		Description: "Search UTF-8 project files for a literal query.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema: &schema.ToolInfo{Name: "search_code", Desc: "Search UTF-8 project files for a literal query.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query":       {Type: schema.String, Desc: "Literal text to search for.", Required: true},
			"path":        {Type: schema.String, Desc: "Optional project-relative directory."},
			"max_matches": {Type: schema.Integer, Desc: "Maximum matches. Defaults to 50."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			_ = ctx
			var input struct {
				Query      string `json:"query"`
				Path       string `json:"path"`
				MaxMatches int    `json:"max_matches"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			if strings.TrimSpace(input.Query) == "" {
				return ToolResult{}, fmt.Errorf("query is required")
			}
			root, err := cleanRoot(runtime.ProjectRoot)
			if err != nil {
				return ToolResult{}, err
			}
			target := root
			if strings.TrimSpace(input.Path) != "" {
				target, err = permission.ValidateProjectPath(root, input.Path)
				if err != nil {
					return ToolResult{}, err
				}
			}
			limit := clamp(input.MaxMatches, 50, 500)
			matches := make([]map[string]any, 0, limit)
			err = filepath.WalkDir(target, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if d.IsDir() && shouldSkipDir(d.Name()) && path != target {
					return filepath.SkipDir
				}
				if d.IsDir() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil || !utf8.Valid(data) {
					return nil
				}
				lines := strings.Split(string(data), "\n")
				for i, line := range lines {
					if strings.Contains(line, input.Query) {
						rel, _ := filepath.Rel(root, path)
						matches = append(matches, map[string]any{"path": filepath.ToSlash(rel), "line": i + 1, "text": truncate(strings.TrimSpace(line), 240)})
						if len(matches) >= limit {
							return errStopWalk
						}
					}
				}
				return nil
			})
			if err != nil && err != errStopWalk {
				return ToolResult{}, err
			}
			out, _ := json.Marshal(map[string]any{"matches": matches, "truncated": len(matches) >= limit})
			return ToolResult{Content: string(out)}, nil
		},
	}
}

func ApplyPatchToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "apply_patch",
		Description: "Replace exact text in a project file after validating the patch in memory; successful writes use the permission executor.",
		Toolset:     DefaultToolset,
		Risk:        "high",
		Schema: &schema.ToolInfo{Name: "apply_patch", Desc: "Replace exact text in a project file after validating the patch in memory; successful writes use the permission executor.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":        {Type: schema.String, Desc: "Project-relative file path.", Required: true},
			"old":         {Type: schema.String, Desc: "Exact text to replace.", Required: true},
			"new":         {Type: schema.String, Desc: "Replacement text.", Required: true},
			"replace_all": {Type: schema.Boolean, Desc: "Replace all occurrences instead of exactly one."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			var input struct {
				Path       string `json:"path"`
				Old        string `json:"old"`
				New        string `json:"new"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			target, err := permission.ValidateProjectPath(runtime.ProjectRoot, input.Path)
			if err != nil {
				return ToolResult{}, err
			}
			data, err := os.ReadFile(target)
			if err != nil {
				return ToolResult{}, err
			}
			content := string(data)
			count := strings.Count(content, input.Old)
			if input.Old == "" || count == 0 {
				return ToolResult{}, fmt.Errorf("patch old text was not found")
			}
			if !input.ReplaceAll && count != 1 {
				return ToolResult{}, fmt.Errorf("patch old text matched %d times; set replace_all=true or make old text unique", count)
			}
			next := strings.Replace(content, input.Old, input.New, 1)
			if input.ReplaceAll {
				next = strings.ReplaceAll(content, input.Old, input.New)
			}
			if runtime.Executor == nil {
				return ToolResult{}, fmt.Errorf("executor is not configured for approved file writes")
			}
			result, err := runtime.Executor.Execute(ctx, Operation{
				Kind:        permission.OperationWriteFile,
				WorkspaceID: runtime.WorkspaceID,
				ProjectRoot: runtime.ProjectRoot,
				Path:        input.Path,
				Content:     next,
				RequestID:   runtime.RequestID,
				PlanStepID:  runtime.PlanStepID,
			}, runtime.SessionID)
			if err != nil {
				return ToolResult{}, err
			}
			out, _ := json.Marshal(map[string]any{"path": input.Path, "replacements": count, "output": result.Output, "approval_id": result.ApprovalID})
			return ToolResult{Content: string(out)}, nil
		},
	}
}

func GlobToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "glob",
		Description: "Find project files matching a glob pattern, similar to Eino DeepAgent filesystem glob.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema: &schema.ToolInfo{Name: "glob", Desc: "Find project files matching a glob pattern, similar to Eino DeepAgent filesystem glob.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern":   {Type: schema.String, Desc: "Glob pattern such as *.go or internal/**/*.go.", Required: true},
			"max_files": {Type: schema.Integer, Desc: "Maximum files to return. Defaults to 200."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			_ = ctx
			var input struct {
				Pattern  string `json:"pattern"`
				MaxFiles int    `json:"max_files"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			if strings.TrimSpace(input.Pattern) == "" {
				return ToolResult{}, fmt.Errorf("pattern is required")
			}
			root, err := cleanRoot(runtime.ProjectRoot)
			if err != nil {
				return ToolResult{}, err
			}
			if strings.Contains(input.Pattern, "..") || filepath.IsAbs(input.Pattern) {
				return ToolResult{}, fmt.Errorf("glob pattern must stay inside project root")
			}
			limit := clamp(input.MaxFiles, 200, 1000)
			files := make([]string, 0, limit)
			err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if d.IsDir() && shouldSkipDir(d.Name()) && path != root {
					return filepath.SkipDir
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return nil
				}
				matched, err := filepath.Match(filepath.FromSlash(input.Pattern), rel)
				if err != nil {
					return err
				}
				if matched {
					files = append(files, filepath.ToSlash(rel))
					if len(files) >= limit {
						return errStopWalk
					}
				}
				return nil
			})
			if err != nil && err != errStopWalk {
				return ToolResult{}, err
			}
			sort.Strings(files)
			out, _ := json.Marshal(map[string]any{"files": files, "truncated": len(files) >= limit})
			return ToolResult{Content: string(out)}, nil
		},
	}
}

func GrepToolSpec() ToolSpec {
	spec := SearchCodeToolSpec()
	spec.Name = "grep"
	spec.Description = "Search project files for literal text, similar to Eino DeepAgent filesystem grep."
	spec.Schema = &schema.ToolInfo{Name: "grep", Desc: spec.Description, ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"query":       {Type: schema.String, Desc: "Literal text to search for.", Required: true},
		"path":        {Type: schema.String, Desc: "Optional project-relative directory."},
		"max_matches": {Type: schema.Integer, Desc: "Maximum matches. Defaults to 50."},
	})}
	return spec
}

func EditFileToolSpec() ToolSpec {
	patch := ApplyPatchToolSpec()
	return ToolSpec{
		Name:        "edit_file",
		Description: "Edit a project file by replacing exact text, similar to Eino DeepAgent filesystem edit_file.",
		Toolset:     DefaultToolset,
		Risk:        "high",
		Schema: &schema.ToolInfo{Name: "edit_file", Desc: "Edit a project file by replacing exact text, similar to Eino DeepAgent filesystem edit_file.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":        {Type: schema.String, Desc: "Project-relative file path.", Required: true},
			"old_string":  {Type: schema.String, Desc: "Exact text to replace.", Required: true},
			"new_string":  {Type: schema.String, Desc: "Replacement text.", Required: true},
			"replace_all": {Type: schema.Boolean, Desc: "Replace all occurrences instead of exactly one."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			var input struct {
				Path       string `json:"path"`
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			mapped, _ := json.Marshal(map[string]any{
				"path":        input.Path,
				"old":         input.OldString,
				"new":         input.NewString,
				"replace_all": input.ReplaceAll,
			})
			return patch.Handler(ctx, mapped, runtime)
		},
	}
}

func ExecuteToolSpec() ToolSpec {
	runShell := RunShellToolSpec()
	return ToolSpec{
		Name:        "execute",
		Description: "Execute a shell command through CodeFlow approval, similar to Eino DeepAgent filesystem execute.",
		Toolset:     DefaultToolset,
		Risk:        "high",
		Schema: &schema.ToolInfo{Name: "execute", Desc: "Execute a shell command through CodeFlow approval, similar to Eino DeepAgent filesystem execute.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command":         {Type: schema.String, Desc: "Shell command to execute.", Required: true},
			"timeout_seconds": {Type: schema.Integer, Desc: "Optional timeout in seconds. Defaults to 60."},
		})},
		Handler: runShell.Handler,
	}
}

func GitStatusToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "git_status",
		Description: "Return porcelain git status for the project root.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema:      &schema.ToolInfo{Name: "git_status", Desc: "Return porcelain git status for the project root."},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			_ = args
			out, err := runDirectCommand(ctx, runtime.ProjectRoot, "git", []string{"status", "--short"}, 20*time.Second)
			if err != nil {
				return ToolResult{}, err
			}
			data, _ := json.Marshal(map[string]any{"dirty": strings.TrimSpace(out) != "", "status": out})
			return ToolResult{Content: string(data)}, nil
		},
	}
}

func GitDiffToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "git_diff",
		Description: "Return git diff for the project root, optionally for one path.",
		Toolset:     DefaultToolset,
		Risk:        "low",
		Schema: &schema.ToolInfo{Name: "git_diff", Desc: "Return git diff for the project root, optionally for one path.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Optional project-relative file path."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			var input struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			gitArgs := []string{"diff", "--"}
			if strings.TrimSpace(input.Path) != "" {
				if _, err := permission.ValidateProjectPath(runtime.ProjectRoot, input.Path); err != nil {
					return ToolResult{}, err
				}
				gitArgs = append(gitArgs, input.Path)
			}
			out, err := runDirectCommand(ctx, runtime.ProjectRoot, "git", gitArgs, 20*time.Second)
			if err != nil {
				return ToolResult{}, err
			}
			data, _ := json.Marshal(map[string]any{"diff": truncate(out, maxToolOutput), "truncated": len(out) > maxToolOutput})
			return ToolResult{Content: string(data)}, nil
		},
	}
}

func RunCheckToolSpec() ToolSpec {
	return ToolSpec{
		Name:        "run_check",
		Description: "Run an approved verification command such as go test, npm test, npm run lint, or npm run build.",
		Toolset:     DefaultToolset,
		Risk:        "medium",
		Schema: &schema.ToolInfo{Name: "run_check", Desc: "Run an approved verification command such as go test, npm test, npm run lint, or npm run build.", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command":         {Type: schema.String, Desc: "Verification command.", Required: true},
			"timeout_seconds": {Type: schema.Integer, Desc: "Optional timeout in seconds. Defaults to 120."},
		})},
		Handler: func(ctx context.Context, args json.RawMessage, runtime ToolRuntime) (ToolResult, error) {
			var input struct {
				Command        string `json:"command"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return ToolResult{}, err
			}
			if err := validateCheckCommand(input.Command); err != nil {
				return ToolResult{}, err
			}
			timeout := clamp(input.TimeoutSeconds, 120, 600)
			out, err := runShellCommand(ctx, runtime.ProjectRoot, input.Command, time.Duration(timeout)*time.Second)
			result := map[string]any{"command": input.Command, "passed": err == nil, "output": truncate(out, maxToolOutput), "truncated": len(out) > maxToolOutput}
			if err != nil {
				result["error"] = err.Error()
			}
			data, _ := json.Marshal(result)
			return ToolResult{Content: string(data)}, nil
		},
	}
}

var errStopWalk = fmt.Errorf("stop walk")

func cleanRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("project root is required")
	}
	return filepath.Abs(root)
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".next", "dist", "build", "vendor", ".cache":
		return true
	default:
		return false
	}
}

func clamp(value, def, max int) int {
	if value <= 0 {
		value = def
	}
	if value > max {
		return max
	}
	return value
}

func validateCheckCommand(command string) error {
	if err := permission.ValidateShellCommand(command); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(command)
	allowed := []string{
		"go test", "go vet", "go version",
		"npm test", "npm run test", "npm run lint", "npm run build",
		"pnpm test", "pnpm run test", "pnpm run lint", "pnpm run build",
	}
	for _, prefix := range allowed {
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+" ") {
			return nil
		}
	}
	return fmt.Errorf("command is not an approved verification command: %s", command)
}

func runDirectCommand(ctx context.Context, dir, name string, args []string, timeout time.Duration) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	if cmdCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out after %s", timeout)
	}
	return out, err
}

func runShellCommand(ctx context.Context, dir, command string, timeout time.Duration) (string, error) {
	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "powershell", "-NoProfile"
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, shell, flag, "-Command", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, shell, flag, command)
	}
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	if cmdCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out after %s", timeout)
	}
	return out, err
}
