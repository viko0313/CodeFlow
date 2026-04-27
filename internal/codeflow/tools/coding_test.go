package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCodingToolsReadSearchAndRejectEscapingPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := NewToolRegistry()
	RegisterCodingTools(registry)
	read, err := registry.Dispatch(context.Background(), toolCall("read_file", `{"path":"main.go"}`), ToolRuntime{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Content, "package main") {
		t.Fatalf("read_file output missing file content: %s", read.Content)
	}
	search, err := registry.Dispatch(context.Background(), toolCall("search_code", `{"query":"func main"}`), ToolRuntime{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(search.Content, "main.go") {
		t.Fatalf("search_code output missing match path: %s", search.Content)
	}
	escaped, err := registry.Dispatch(context.Background(), toolCall("read_file", `{"path":"../outside.txt"}`), ToolRuntime{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(escaped.Content, "path escapes project root") {
		t.Fatalf("expected structured path escape error, got %s", escaped.Content)
	}
}

func TestApplyPatchFailureDoesNotTouchDisk(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := ApplyPatchToolSpec()
	result, err := spec.Handler(context.Background(), json.RawMessage(`{"path":"note.txt","old":"missing","new":"beta"}`), ToolRuntime{ProjectRoot: root})
	if err == nil {
		t.Fatalf("expected patch failure, got result %s", result.Content)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "alpha\n" {
		t.Fatalf("file changed after failed patch: %q", string(data))
	}
}

func TestGitStatusReportsDirtyState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "CodeFlow Test")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := GitStatusToolSpec()
	result, err := spec.Handler(context.Background(), json.RawMessage(`{}`), ToolRuntime{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, `"dirty":true`) {
		t.Fatalf("expected dirty git status, got %s", result.Content)
	}
}

func TestRunCheckExecutesSafeCommandWithSummary(t *testing.T) {
	root := t.TempDir()
	spec := RunCheckToolSpec()
	command := "go version"
	if runtime.GOOS == "windows" {
		command = "go version"
	}
	result, err := spec.Handler(context.Background(), json.RawMessage(`{"command":"`+command+`","timeout_seconds":20}`), ToolRuntime{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, `"passed":true`) || !strings.Contains(result.Content, "go version") {
		t.Fatalf("unexpected run_check result: %s", result.Content)
	}
}

func TestEinoStyleFilesystemToolNamesAreRegistered(t *testing.T) {
	registry := DefaultRegistry()
	defs, err := registry.Definitions(context.Background(), DefaultToolset)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Name] = true
	}
	for _, name := range []string{"read_file", "write_file", "edit_file", "glob", "grep", "execute"} {
		if !names[name] {
			t.Fatalf("expected Eino-style tool %s to be registered; got %+v", name, names)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func toolCall(name, args string) schema.ToolCall {
	return schema.ToolCall{ID: "call_test", Function: schema.FunctionCall{Name: name, Arguments: args}}
}
