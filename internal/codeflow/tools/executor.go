package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cloudwego/codeflow/internal/codeflow/audit"
	"github.com/cloudwego/codeflow/internal/codeflow/permission"
)

type Operation struct {
	Kind        permission.OperationKind
	ProjectRoot string
	Path        string
	Content     string
	Append      bool
	Command     string
	Timeout     time.Duration
}

type Result struct {
	Output    string
	Duration  time.Duration
	Confirmed bool
}

type Executor struct {
	gate    *permission.Gate
	auditor *audit.Logger
}

func NewExecutor(gate *permission.Gate, auditor *audit.Logger) *Executor {
	return &Executor{gate: gate, auditor: auditor}
}

func (e *Executor) Execute(ctx context.Context, op Operation, sessionID string) (Result, error) {
	start := time.Now()
	id := "op_" + uuid.NewString()[:8]
	preview := ""
	risk := "medium"
	if op.Kind == permission.OperationWriteFile {
		target, err := permission.ValidateProjectPath(op.ProjectRoot, op.Path)
		if err != nil {
			return Result{}, err
		}
		preview = buildDiff(target, op.Content, op.Append)
	}
	if op.Kind == permission.OperationShell {
		if op.Timeout <= 0 {
			op.Timeout = 60 * time.Second
		}
		preview = fmt.Sprintf("$ %s", op.Command)
		risk = shellRisk(op.Command)
	}
	decision, err := e.gate.Review(ctx, permission.Operation{
		ID:          id,
		Kind:        op.Kind,
		ProjectRoot: op.ProjectRoot,
		Path:        op.Path,
		Command:     op.Command,
		Preview:     preview,
		Risk:        risk,
		Timeout:     op.Timeout.String(),
	})
	if err != nil {
		return Result{}, err
	}
	confirmed := decision.Allowed
	defer e.record(sessionID, op, id, start, confirmed, decision.Reason)
	if !decision.Allowed {
		return Result{Confirmed: false, Duration: time.Since(start)}, fmt.Errorf("operation denied: %s", decision.Reason)
	}
	switch op.Kind {
	case permission.OperationWriteFile:
		out, err := e.writeFile(op)
		return Result{Output: out, Duration: time.Since(start), Confirmed: true}, err
	case permission.OperationShell:
		out, err := e.shell(ctx, op)
		return Result{Output: out, Duration: time.Since(start), Confirmed: true}, err
	default:
		return Result{}, fmt.Errorf("unsupported operation kind: %s", op.Kind)
	}
}

func (e *Executor) writeFile(op Operation) (string, error) {
	target, err := permission.ValidateProjectPath(op.ProjectRoot, op.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	flag := os.O_CREATE | os.O_WRONLY
	if op.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(target, flag, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(op.Content); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s", op.Path), nil
}

func (e *Executor) shell(ctx context.Context, op Operation) (string, error) {
	if err := permission.ValidateShellCommand(op.Command); err != nil {
		return "", err
	}
	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "powershell", "-NoProfile"
	}
	cmdCtx, cancel := context.WithTimeout(ctx, op.Timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, shell, flag, "-Command", op.Command)
	} else {
		cmd = exec.CommandContext(cmdCtx, shell, flag, op.Command)
	}
	cmd.Dir = op.ProjectRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := truncate(stdout.String()+stderr.String(), 8000)
	if cmdCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out after %s", op.Timeout)
	}
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(out) == "" {
		return "command completed with no output", nil
	}
	return out, nil
}

func (e *Executor) record(sessionID string, op Operation, operationID string, start time.Time, confirmed bool, result string) {
	if e.auditor == nil {
		return
	}
	_ = e.auditor.Record(audit.Event{
		SessionID:     sessionID,
		ProjectRoot:   op.ProjectRoot,
		OperationID:   operationID,
		Event:         string(op.Kind),
		ToolName:      string(op.Kind),
		ArgsSummary:   argsSummary(op),
		ResultSummary: truncate(result, 200),
		DurationMS:    time.Since(start).Milliseconds(),
		Confirmed:     &confirmed,
	})
}

func buildDiff(path, newContent string, appendMode bool) string {
	old := ""
	if data, err := os.ReadFile(path); err == nil {
		old = string(data)
	}
	if appendMode {
		newContent = old + newContent
	}
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(newContent, "\n")
	var b strings.Builder
	b.WriteString("--- before\n+++ after\n")
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	for i := 0; i < max; i++ {
		var o, n string
		if i < len(oldLines) {
			o = oldLines[i]
		}
		if i < len(newLines) {
			n = newLines[i]
		}
		if o == n {
			continue
		}
		if i < len(oldLines) {
			b.WriteString("-" + o + "\n")
		}
		if i < len(newLines) {
			b.WriteString("+" + n + "\n")
		}
	}
	return b.String()
}

func shellRisk(command string) string {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "git push") || strings.Contains(lower, "curl ") || strings.Contains(lower, "invoke-restmethod") {
		return "high"
	}
	return "medium"
}

func argsSummary(op Operation) string {
	if op.Kind == permission.OperationShell {
		return op.Command
	}
	if op.Path != "" {
		return op.Path
	}
	return string(op.Kind)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
