package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigExpandsEnvAndArkDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARK_API_KEY", "test-key")
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("api_key: ${ARK_API_KEY}\nworkspace: ./workspace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("expected expanded api key, got %q", cfg.APIKey)
	}
	if cfg.Provider != "ark" || cfg.Model != "doubao-seed-1-8-251228" {
		t.Fatalf("unexpected defaults: %s/%s", cfg.Provider, cfg.Model)
	}
}

func TestLoadConfigLocalOverridesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	if err := os.WriteFile("config.yaml", []byte("provider: openai\nmodel: base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("config.local.yaml", []byte("provider: ark\nmodel: local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "ark" || cfg.Model != "local" {
		t.Fatalf("local config did not override defaults: %s/%s", cfg.Provider, cfg.Model)
	}
}

func TestLoadConfigOllamaDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("provider: ollama\nworkspace: ./workspace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "qwen3.5:4b" {
		t.Fatalf("expected ollama qwen default, got %q", cfg.Model)
	}
	if cfg.BaseURL != "http://localhost:11434" {
		t.Fatalf("expected ollama base url, got %q", cfg.BaseURL)
	}
	if cfg.APIKey != "" {
		t.Fatalf("ollama should not require api key, got %q", cfg.APIKey)
	}
}
