package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Preloaded   bool   `json:"preloaded"`
	Content     string `json:"content,omitempty"`
}

type Manifest struct {
	Enabled bool     `json:"enabled"`
	Dirs    []string `json:"dirs"`
	Skills  []Skill  `json:"skills"`
}

func Load(cfg cfconfig.SkillsConfig) (Manifest, error) {
	manifest := Manifest{Enabled: cfg.Enabled, Dirs: cfg.Dirs}
	if !cfg.Enabled {
		return manifest, nil
	}
	preload := map[string]bool{}
	for _, name := range cfg.Preload {
		preload[strings.ToLower(strings.TrimSpace(name))] = true
	}
	for _, dir := range cfg.Dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return manifest, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			doc := filepath.Join(dir, entry.Name(), "SKILL.md")
			if _, err := os.Stat(doc); err != nil {
				doc = filepath.Join(dir, entry.Name(), "README.md")
			}
			data, err := os.ReadFile(doc)
			if err != nil {
				continue
			}
			content := string(data)
			name := metadata(content, "name", entry.Name())
			key := strings.ToLower(name)
			allPreloaded := len(preload) == 0 && len(cfg.Preload) > 0 && cfg.Preload[0] == "*"
			isPreloaded := allPreloaded || preload[key] || preload[strings.ToLower(entry.Name())]
			skill := Skill{
				Name:        name,
				Description: metadata(content, "description", "CodeFlow skill "+entry.Name()),
				Path:        doc,
				Preloaded:   isPreloaded,
			}
			if isPreloaded {
				skill.Content = truncate(content, cfg.MaxContentBytes)
			}
			manifest.Skills = append(manifest.Skills, skill)
		}
	}
	sort.Slice(manifest.Skills, func(i, j int) bool {
		return manifest.Skills[i].Name < manifest.Skills[j].Name
	})
	return manifest, nil
}

func PreloadText(manifest Manifest) string {
	if !manifest.Enabled {
		return ""
	}
	var b strings.Builder
	for _, skill := range manifest.Skills {
		if !skill.Preloaded || strings.TrimSpace(skill.Content) == "" {
			continue
		}
		b.WriteString("## ")
		b.WriteString(skill.Name)
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(skill.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func metadata(content, key, fallback string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s*(.+)$`)
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return fallback
	}
	value := strings.Trim(strings.TrimSpace(match[1]), `"'`)
	if value == "" {
		return fallback
	}
	return value
}

func truncate(content string, max int) string {
	if max <= 0 || len(content) <= max {
		return content
	}
	return content[:max] + "\n\n...[skill truncated]..."
}
