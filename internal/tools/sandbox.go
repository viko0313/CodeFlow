package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type SandboxConfig struct {
	OfficeDir string
}

type SandboxTool struct {
	config SandboxConfig
}

func NewSandboxTool(officeDir string) *SandboxTool {
	return &SandboxTool{
		config: SandboxConfig{OfficeDir: officeDir},
	}
}

func (s *SandboxTool) validatePath(relativePath string) (string, error) {
	baseDir, err := filepath.Abs(s.config.OfficeDir)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(relativePath) || hasWindowsDrive(relativePath) {
		return "", errors.New("access denied: absolute paths are not allowed")
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." {
		cleaned = ""
	}
	targetPath := filepath.Join(baseDir, cleaned)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseDir, absTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", errors.New("access denied: path outside sandbox")
	}
	return absTarget, nil
}

func hasWindowsDrive(path string) bool {
	return len(path) >= 2 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':'
}

type ListFilesTool struct {
	*SandboxTool
}

func NewListFilesTool(officeDir string) *ListFilesTool {
	return &ListFilesTool{SandboxTool: NewSandboxTool(officeDir)}
}

func (t *ListFilesTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_office_files",
		Desc: "List files and directories in the office sandbox. Use empty string for root directory.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"sub_dir": {
				Type: schema.String,
				Desc: "Subdirectory relative to office root",
			},
		}),
	}, nil
}

func (t *ListFilesTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	return t.SandboxTool.ListFiles(ctx, argsInJSON)
}

func (t *SandboxTool) ListFiles(ctx context.Context, argsInJSON string) (string, error) {
	var args struct {
		SubDir string `json:"sub_dir"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	targetDir, err := t.validatePath(args.SubDir)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	result := ""
	for _, entry := range entries {
		icon := "file"
		if entry.IsDir() {
			icon = "dir"
		}
		result += fmt.Sprintf("[%s] %s\n", icon, entry.Name())
	}
	return result, nil
}

type ReadFileTool struct {
	*SandboxTool
}

func NewReadFileTool(officeDir string) *ReadFileTool {
	return &ReadFileTool{SandboxTool: NewSandboxTool(officeDir)}
}

func (t *ReadFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_office_file",
		Desc: "Read content of a file in the office sandbox. Use relative path from office root.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"filepath": {
				Type:     schema.String,
				Desc:     "Relative path to file",
				Required: true,
			},
		}),
	}, nil
}

func (t *ReadFileTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	return t.SandboxTool.ReadFile(ctx, argsInJSON)
}

func (t *SandboxTool) ReadFile(ctx context.Context, argsInJSON string) (string, error) {
	var args struct {
		Filepath string `json:"filepath"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	targetPath, err := t.validatePath(args.Filepath)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	result := string(content)
	if len(result) > 10000 {
		result = result[:10000] + "\n\n...[content truncated]..."
	}
	return result, nil
}

type WriteFileTool struct {
	*SandboxTool
}

func NewWriteFileTool(officeDir string) *WriteFileTool {
	return &WriteFileTool{SandboxTool: NewSandboxTool(officeDir)}
}

func (t *WriteFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_office_file",
		Desc: "Write content to a file in the office sandbox. Supports 'w' (overwrite) and 'a' (append) modes.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"filepath": {
				Type:     schema.String,
				Desc:     "Relative path to file",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "Content to write",
				Required: true,
			},
			"mode": {
				Type: schema.String,
				Desc: "Write mode: 'w' for overwrite, 'a' for append",
			},
		}),
	}, nil
}

func (t *WriteFileTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	return t.SandboxTool.WriteFile(ctx, argsInJSON)
}

func (t *SandboxTool) WriteFile(ctx context.Context, argsInJSON string) (string, error) {
	var args struct {
		Filepath string `json:"filepath"`
		Content  string `json:"content"`
		Mode     string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Mode == "" {
		args.Mode = "w"
	}
	if args.Mode != "w" && args.Mode != "a" {
		return "", fmt.Errorf("invalid mode: must be 'w' or 'a'")
	}

	targetPath, err := t.validatePath(args.Filepath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directory: %w", err)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if args.Mode == "a" {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(targetPath, flag, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(args.Content)
	if err != nil {
		return "", fmt.Errorf("failed to write: %w", err)
	}

	return fmt.Sprintf("Successfully wrote to %s (mode: %s)", args.Filepath, args.Mode), nil
}

type ExecuteShellTool struct {
	*SandboxTool
}

func NewExecuteShellTool(officeDir string) *ExecuteShellTool {
	return &ExecuteShellTool{SandboxTool: NewSandboxTool(officeDir)}
}

func (t *ExecuteShellTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "execute_office_shell",
		Desc: "Execute shell commands in the office sandbox. All operations are restricted to the sandbox directory.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {
				Type:     schema.String,
				Desc:     "Shell command to execute",
				Required: true,
			},
		}),
	}, nil
}

func (t *ExecuteShellTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	return t.SandboxTool.ExecuteShell(ctx, argsInJSON)
}

func (t *SandboxTool) ExecuteShell(ctx context.Context, argsInJSON string) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if err := validateShellCommand(args.Command); err != nil {
		return "", err
	}

	var shell, flag string

	switch runtime.GOOS {
	case "windows":
		shell = "cmd.exe"
		flag = "/C"
	case "darwin", "linux":
		shell = "/bin/sh"
		flag = "-c"
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, shell, flag, args.Command)
	cmd.Dir = t.config.OfficeDir

	output, err := cmd.CombinedOutput()
	out := truncateOutput(string(output), 4000)
	if cmdCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out after 60s")
	}
	if err != nil {
		return out, fmt.Errorf("command failed: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Sprintf("Command completed successfully on %s with no output.", runtime.GOOS), nil
	}
	return out, nil
}

func validateShellCommand(command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("empty command")
	}
	patterns := []string{
		`\.\.`,
		`(?i)(^|\s)cd(\s|$)`,
		`(?i)(^|\s)(python|python3|node)\s+(-c|-e)`,
		`(^|\s|[<>|&;])/`,
		`(^|\s|[<>|&;])~`,
		`(?i)(^|\s|[<>|&;])[a-z]:`,
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).FindString(command) != "" {
			return errors.New("system blocked: violation of CodeFlow security protocol")
		}
	}
	return nil
}

func truncateOutput(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n\n...[output truncated]..."
}
