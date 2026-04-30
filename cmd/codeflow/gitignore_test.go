package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitignoreCoversLocalRuntimeSecrets(t *testing.T) {
	paths := []string{
		".codeflow/secret.key",
		".codeflow/codeflow.db",
		".codeflow/web-api.err.log",
		".codeflow/dev-logs/server.log",
	}
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, path := range paths {
		cmd := exec.Command("git", "check-ignore", path)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("expected %s to be ignored, output=%s err=%v", path, out, err)
		}
	}
}
