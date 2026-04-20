package cli

import "testing"

func TestExecuteVersionDoesNotRequireServices(t *testing.T) {
	if err := Execute([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteConfigSetGetDoesNotRequireServices(t *testing.T) {
	dir := t.TempDir()
	if err := Execute([]string{"--project-root", dir, "config", "set", "provider", "ollama"}); err != nil {
		t.Fatal(err)
	}
	if err := Execute([]string{"--project-root", dir, "config", "get", "provider"}); err != nil {
		t.Fatal(err)
	}
}
