package permission

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type OperationKind string

const (
	OperationWriteFile OperationKind = "write_file"
	OperationShell     OperationKind = "shell"
	OperationExternal  OperationKind = "external_write"
)

type Operation struct {
	ID          string
	ApprovalID  string
	RequestID   string
	Kind        OperationKind
	ProjectRoot string
	Path        string
	Command     string
	Preview     string
	Risk        string
	Timeout     string
}

type Decision struct {
	Allowed bool
	Reason  string
}

type Confirmer func(ctx context.Context, op Operation) (Decision, error)

type Gate struct {
	trustedCommands []string
	trustedDirs     []string
	writableDirs    []string
	forceApproval   bool
	confirm         Confirmer
}

type Options struct {
	TrustedCommands []string
	TrustedDirs     []string
	WritableDirs    []string
	ForceApproval   bool
	Confirmer       Confirmer
}

func NewGate(opts Options) *Gate {
	confirm := opts.Confirmer
	if confirm == nil {
		confirm = TerminalConfirmer(os.Stdin, os.Stdout)
	}
	return &Gate{
		trustedCommands: opts.TrustedCommands,
		trustedDirs:     opts.TrustedDirs,
		writableDirs:    opts.WritableDirs,
		forceApproval:   opts.ForceApproval,
		confirm:         confirm,
	}
}

func (g *Gate) Review(ctx context.Context, op Operation) (Decision, error) {
	if strings.TrimSpace(op.ProjectRoot) == "" {
		return Decision{}, errors.New("project root is required")
	}
	if op.Kind == OperationWriteFile {
		if _, err := ValidateProjectPath(op.ProjectRoot, op.Path); err != nil {
			return Decision{Allowed: false, Reason: err.Error()}, nil
		}
		if len(g.writableDirs) > 0 && !withinAny(op.ProjectRoot, op.Path, g.writableDirs) {
			return Decision{Allowed: false, Reason: "path is outside configured writable directories"}, nil
		}
	}
	if op.Kind == OperationShell {
		if err := ValidateShellCommand(op.Command); err != nil {
			return Decision{Allowed: false, Reason: err.Error()}, nil
		}
		if !g.forceApproval && g.commandTrusted(op.Command) {
			return Decision{Allowed: true, Reason: "trusted command"}, nil
		}
	}
	if op.Kind == OperationWriteFile && !g.forceApproval && withinAny(op.ProjectRoot, op.Path, g.trustedDirs) {
		return Decision{Allowed: true, Reason: "trusted directory"}, nil
	}
	return g.confirm(ctx, op)
}

func TerminalConfirmer(in *os.File, out *os.File) Confirmer {
	return func(ctx context.Context, op Operation) (Decision, error) {
		fmt.Fprintf(out, "\n[CodeFlow permission]\n")
		fmt.Fprintf(out, "Operation: %s\n", op.Kind)
		if op.Path != "" {
			fmt.Fprintf(out, "Path: %s\n", op.Path)
		}
		if op.Command != "" {
			fmt.Fprintf(out, "Command: %s\n", op.Command)
			fmt.Fprintf(out, "Working directory: %s\n", op.ProjectRoot)
		}
		if op.Timeout != "" {
			fmt.Fprintf(out, "Timeout: %s\n", op.Timeout)
		}
		if op.Risk != "" {
			fmt.Fprintf(out, "Risk: %s\n", op.Risk)
		}
		if strings.TrimSpace(op.Preview) != "" {
			fmt.Fprintf(out, "\nPreview:\n%s\n", op.Preview)
		}
		fmt.Fprint(out, "Approve? [y/N]: ")
		ch := make(chan string, 1)
		go func() {
			reader := bufio.NewReader(in)
			text, _ := reader.ReadString('\n')
			ch <- strings.TrimSpace(strings.ToLower(text))
		}()
		select {
		case <-ctx.Done():
			return Decision{Allowed: false, Reason: "cancelled"}, ctx.Err()
		case answer := <-ch:
			if answer == "y" || answer == "yes" {
				return Decision{Allowed: true, Reason: "user approved"}, nil
			}
			return Decision{Allowed: false, Reason: "user denied"}, nil
		}
	}
}

func ValidateProjectPath(projectRoot, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(relPath) || hasWindowsDrive(relPath) {
		return "", errors.New("absolute paths are not allowed")
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(relPath)
	target, err := filepath.Abs(filepath.Join(root, clean))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", errors.New("path escapes project root")
	}
	return target, nil
}

func ValidateShellCommand(command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("empty command")
	}
	patterns := []string{
		`(?i)(^|\s)(rm|del|Remove-Item)\s+(-rf|-r|-Recurse)`,
		`(?i)(^|\s)(python|python3|node)\s+(-c|-e)`,
		`(?i)(^|\s)cd\s+\.\.`,
		`(^|\s|[<>|&;])\.\.([/\\]|$)`,
	}
	if runtime.GOOS == "windows" {
		patterns = append(patterns, `(?i)(^|\s|[<>|&;])[a-z]:\\windows`)
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).FindString(command) != "" {
			return errors.New("system blocked: violation of CodeFlow security protocol")
		}
	}
	return nil
}

func (g *Gate) commandTrusted(command string) bool {
	trimmed := strings.TrimSpace(command)
	for _, trusted := range g.trustedCommands {
		trusted = strings.TrimSpace(trusted)
		if trusted != "" && (trimmed == trusted || strings.HasPrefix(trimmed, trusted+" ")) {
			return true
		}
	}
	return false
}

func withinAny(projectRoot, relPath string, dirs []string) bool {
	if len(dirs) == 0 {
		return false
	}
	target, err := ValidateProjectPath(projectRoot, relPath)
	if err != nil {
		return false
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return false
	}
	for _, dir := range dirs {
		base, err := filepath.Abs(filepath.Join(root, filepath.Clean(dir)))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, target)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func hasWindowsDrive(path string) bool {
	return len(path) >= 2 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':'
}
