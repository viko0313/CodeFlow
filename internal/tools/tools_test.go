package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSandboxPathValidationAndInvokableTools(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	read := NewReadFileTool(dir)
	out, err := read.InvokableRun(ctx, `{"filepath":"hello.txt"}`)
	if err != nil || out != "hello" {
		t.Fatalf("read failed: out=%q err=%v", out, err)
	}
	if _, err := read.InvokableRun(ctx, `{"filepath":"../secret.txt"}`); err == nil {
		t.Fatal("expected traversal to be blocked")
	}
	if _, err := read.InvokableRun(ctx, `{"filepath":"C:\\secret.txt"}`); err == nil {
		t.Fatal("expected drive path to be blocked")
	}

	write := NewWriteFileTool(dir)
	if _, err := write.InvokableRun(ctx, `{"filepath":"nested/out.txt","content":"ok","mode":"w"}`); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "nested", "out.txt")); err != nil || string(data) != "ok" {
		t.Fatalf("write failed: %q %v", data, err)
	}

	list := NewListFilesTool(dir)
	if out, err := list.InvokableRun(ctx, `{}`); err != nil || !strings.Contains(out, "hello.txt") {
		t.Fatalf("list failed: %q %v", out, err)
	}
}

func TestShellBlocksDangerousCommands(t *testing.T) {
	shell := NewExecuteShellTool(t.TempDir())
	if _, err := shell.InvokableRun(context.Background(), `{"command":"cd .."}`); err == nil {
		t.Fatal("expected cd .. to be blocked")
	}
}

func TestCalculator(t *testing.T) {
	calc := NewCalculatorTool()
	out, err := calc.InvokableRun(context.Background(), `{"expression":"(2 + 3) * 4 / 2"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "= 10") {
		t.Fatalf("unexpected calculator output: %s", out)
	}
	if _, err := calc.InvokableRun(context.Background(), `{"expression":"__import__(\"os\")"}`); err == nil {
		t.Fatal("expected unsafe expression to fail")
	}
}

func TestProfileTools(t *testing.T) {
	dir := t.TempDir()
	save := NewSaveProfileTool(dir)
	read := NewReadProfileTool(dir)
	if _, err := save.InvokableRun(context.Background(), `{"new_content":"# Profile\nlikes Go"}`); err != nil {
		t.Fatal(err)
	}
	out, err := read.InvokableRun(context.Background(), `{}`)
	if err != nil || !strings.Contains(out, "likes Go") {
		t.Fatalf("profile read failed: %q %v", out, err)
	}
}

func TestTaskToolsAndDynamicSkill(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	tasksFile := filepath.Join(dir, "tasks.json")
	schedule := NewScheduleTaskTool(tasksFile)
	if _, err := schedule.InvokableRun(ctx, `{"target_time":"2099-01-01 08:00:00","description":"drink water","repeat":"daily","repeat_count":2}`); err != nil {
		t.Fatal(err)
	}
	list := NewListScheduledTasksTool(tasksFile)
	out, err := list.InvokableRun(ctx, `{}`)
	if err != nil || !strings.Contains(out, "drink water") {
		t.Fatalf("task list failed: %q %v", out, err)
	}
	var tasks []ScheduledTask
	data, _ := os.ReadFile(tasksFile)
	if err := json.Unmarshal(data, &tasks); err != nil || len(tasks) != 1 {
		t.Fatalf("bad tasks json: %v len=%d", err, len(tasks))
	}
	modify := NewModifyScheduledTaskTool(tasksFile)
	if _, err := modify.InvokableRun(ctx, `{"task_id":"`+tasks[0].ID+`","new_description":"stretch"}`); err != nil {
		t.Fatal(err)
	}
	deleteTool := NewDeleteScheduledTaskTool(tasksFile)
	if _, err := deleteTool.InvokableRun(ctx, `{"task_id":"`+tasks[0].ID+`"}`); err != nil {
		t.Fatal(err)
	}

	office := filepath.Join(dir, "office")
	skills := filepath.Join(office, "skills", "demo")
	if err := os.MkdirAll(skills, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skills, "SKILL.md"), []byte("name: demo_skill\ndescription: Demo skill\nrun me"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDynamicSkillTools(filepath.Join(office, "skills"), office)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("skill load failed: %v len=%d", err, len(loaded))
	}
	inv := loaded[0].(*DynamicSkillTool)
	help, err := inv.InvokableRun(ctx, `{"mode":"help"}`)
	if err != nil || !strings.Contains(help, "run me") {
		t.Fatalf("skill help failed: %q %v", help, err)
	}
}
