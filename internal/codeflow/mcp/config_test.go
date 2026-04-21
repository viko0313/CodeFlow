package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
)

func TestLoadMCPServersFromClaudeStyleJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	content := `{"mcpServers":{"fs":{"command":"node","args":["server.js"],"env":{"ROOT":"."}}}}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Load(cfconfig.MCPConfig{Enabled: true, Preload: true, ConfigFiles: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Servers) != 1 || manifest.Servers[0].Name != "fs" || !manifest.Servers[0].Preloaded {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if text := PreloadText(manifest); !strings.Contains(text, "node server.js") {
		t.Fatalf("unexpected preload text: %q", text)
	}
}
