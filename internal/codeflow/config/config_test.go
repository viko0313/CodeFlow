package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAndLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("CODEFLOW_POSTGRES_DSN", "postgres://user:pass@localhost:5432/codeflow")
	if err := EnsureProjectConfig(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectRoot != dir {
		t.Fatalf("project root mismatch: %s", cfg.ProjectRoot)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("expected env-expanded api key, got %q", cfg.APIKey)
	}
	if cfg.Storage.PostgresDSN == "" {
		t.Fatal("expected postgres dsn from env")
	}
}

func TestRejectPlaintextSecretInProjectConfig(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".codeflow")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("api_key: sk-realish-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected plaintext api_key to be rejected")
	}
}

func TestConfigSetRequiresSecretEnvReference(t *testing.T) {
	dir := t.TempDir()
	if err := Set(dir, "api_key", "plain-secret"); err == nil {
		t.Fatal("expected plaintext api_key set to fail")
	}
	if err := Set(dir, "provider", "ollama"); err != nil {
		t.Fatal(err)
	}
	value, err := Get(dir, "provider")
	if err != nil {
		t.Fatal(err)
	}
	if value != "ollama" {
		t.Fatalf("expected provider ollama, got %q", value)
	}
}
