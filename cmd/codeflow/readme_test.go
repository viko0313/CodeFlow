package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReadmeDocumentsCodeFlowPhase1(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	for _, want := range []string{
		"CodeFlow Agent",
		"go run .\\cmd\\codeflow",
		"codeflow start",
		"PostgreSQL",
		"Redis",
		"AGENT.md",
		"Permission",
		"go test ./...",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README missing %q", want)
		}
	}
	secretPattern := regexp.MustCompile(`(?i)(sk-[a-z0-9]{20,}|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)
	if secretPattern.MatchString(readme) {
		t.Fatalf("README appears to contain a real API key or UUID-style secret")
	}
}
