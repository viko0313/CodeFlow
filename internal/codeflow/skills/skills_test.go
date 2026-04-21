package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
)

func TestLoadPreloadedSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: demo\ndescription: Demo skill\n\nUse this skill."), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Load(cfconfig.SkillsConfig{Enabled: true, Dirs: []string{filepath.Join(dir, "skills")}, Preload: []string{"demo"}, MaxContentBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Skills) != 1 || !manifest.Skills[0].Preloaded {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if text := PreloadText(manifest); !strings.Contains(text, "Use this skill") {
		t.Fatalf("preload text missing skill content: %q", text)
	}
}
