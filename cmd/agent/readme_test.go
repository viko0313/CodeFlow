package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReadmeDocumentsGoEdition(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	for _, want := range []string{
		"CyberClaw-Go",
		"go run .\\cmd\\agent",
		"provider: \"ollama\"",
		"qwen3.5:4b",
		"provider: \"qwen\"",
		"安全沙盒",
		"Tokens",
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
