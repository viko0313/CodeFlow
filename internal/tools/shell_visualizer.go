package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/cloudwego/codeflow/internal/config"
)

type ToolInfo struct {
	Name string `json:"name"`
}

type ShellOutput struct {
	ExecutionID   string        `json:"execution_id"`
	ExecutionTime time.Time     `json:"execution_time"`
	Command       string        `json:"command"`
	CommandOutput []byte        `json:"command_output"`
	ExitCode      int           `json:"exit_code"`
	Stdout        string        `json:"stdout"`
	Stderr        string        `json:"stderr"`
	StartTime     time.Time     `json:"start_time"`
	ElapsedTime   time.Duration `json:"elapsed_time"`
	ShellCommand  string        `json:"shell_command"`
	Environment   []string      `json:"environment"`
	Timestamp     time.Time     `json:"timestamp"`
}

type ShellVisualizer struct {
	ctx          context.Context
	cfg          *config.Config
	mu           sync.Mutex
	history      []ShellOutput
	maxHistory   int
	maxOutputLen int
}

func NewShellVisualizer(cfg *config.Config) *ShellVisualizer {
	return &ShellVisualizer{cfg: cfg, maxHistory: 10, maxOutputLen: 5000}
}

func (t *ShellVisualizer) GetToolInfo(name string) *ToolInfo { return &ToolInfo{Name: name} }

func (t *ShellVisualizer) shellCommand() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
}

func (t *ShellVisualizer) shellArgs(cmd string) []string {
	if runtime.GOOS == "windows" {
		return []string{"-NoProfile", "-Command", cmd}
	}
	return []string{"-c", cmd}
}

func (t *ShellVisualizer) ExecuteShell(cmd string) (string, error) {
	start := time.Now()
	command := exec.Command(t.shellCommand(), t.shellArgs(cmd)...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	out := ShellOutput{
		ExecutionID:   fmt.Sprintf("%d", start.UnixNano()),
		ExecutionTime: start,
		Command:       cmd,
		CommandOutput: stdout.Bytes(),
		ExitCode:      exitCode,
		Stdout:        trimOutput(stdout.String(), t.maxOutputLen),
		Stderr:        trimOutput(stderr.String(), t.maxOutputLen),
		StartTime:     start,
		ElapsedTime:   time.Since(start),
		ShellCommand:  t.shellCommand(),
		Timestamp:     time.Now(),
	}

	t.mu.Lock()
	t.history = append([]ShellOutput{out}, t.history...)
	if t.maxHistory > 0 && len(t.history) > t.maxHistory {
		t.history = t.history[:t.maxHistory]
	}
	t.mu.Unlock()

	if err != nil {
		return out.Stdout + out.Stderr, err
	}
	return out.Stdout, nil
}

func (t *ShellVisualizer) ExecuteStdout(cmd string) (string, error) { return t.ExecuteShell(cmd) }
func (t *ShellVisualizer) ExecuteStderr(cmd string) (string, error) { return t.ExecuteShell(cmd) }

func (t *ShellVisualizer) Edit(cmd string) (string, string, error) {
	out, err := t.ExecuteShell(cmd)
	return out, "", err
}

func (t *ShellVisualizer) EditStdout(cmd string) (string, string, error) {
	out, err := t.ExecuteShell(cmd)
	return out, "", err
}

func (t *ShellVisualizer) EditStderr(cmd string) (string, string, error) {
	out, err := t.ExecuteShell(cmd)
	return "", out, err
}

func (t *ShellVisualizer) Execute(cmd string) (string, error) {
	return t.ExecuteShell(cmd)
}

func (t *ShellVisualizer) ExecuteWithColors(cmd string) (string, error) {
	return t.ExecuteShell(cmd)
}

func (t *ShellVisualizer) Complete(parts []string) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	prefix := parts[len(parts)-1]
	switch prefix {
	case "ls":
		return "ls -la", nil
	case "cd":
		return "cd .", nil
	case "git":
		return "git status", nil
	default:
		return prefix, nil
	}
}

func (t *ShellVisualizer) GetHistory(limit int) []ShellOutput {
	t.mu.Lock()
	defer t.mu.Unlock()
	if limit <= 0 || limit > len(t.history) {
		limit = len(t.history)
	}
	out := make([]ShellOutput, limit)
	copy(out, t.history[:limit])
	return out
}

func (t *ShellVisualizer) ClearHistory(maxAge time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if maxAge <= 0 {
		t.history = nil
		return
	}
	cutoff := time.Now().Add(-maxAge)
	dst := t.history[:0]
	for _, item := range t.history {
		if item.Timestamp.After(cutoff) {
			dst = append(dst, item)
		}
	}
	t.history = dst
}

func (t *ShellVisualizer) FormatOutput(output ShellOutput, err error) string {
	status := "Success"
	if err != nil {
		status = "Error"
	}
	return fmt.Sprintf(
		"Status: %s\nTime: %s\nCommand: %s\nExit Code: %d\nOutput:\n%s\n",
		status,
		output.ElapsedTime,
		output.Command,
		output.ExitCode,
		trimOutput(output.Stdout+"\n"+output.Stderr, t.maxOutputLen),
	)
}

func (t *ShellVisualizer) FormatOutputJSON(output ShellOutput, err error) string {
	payload := map[string]any{
		"status":    "Success",
		"time":      output.ElapsedTime.String(),
		"command":   output.Command,
		"exit_code": output.ExitCode,
		"stdout":    output.Stdout,
		"stderr":    output.Stderr,
	}
	if err != nil {
		payload["status"] = "Error"
		payload["error"] = err.Error()
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func trimOutput(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
