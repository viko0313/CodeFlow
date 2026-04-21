package mcp

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	"go.yaml.in/yaml/v3"
)

type Server struct {
	Name      string            `json:"name" yaml:"name"`
	Command   string            `json:"command" yaml:"command"`
	Args      []string          `json:"args" yaml:"args"`
	Env       map[string]string `json:"env,omitempty" yaml:"env"`
	Enabled   bool              `json:"enabled" yaml:"enabled"`
	Source    string            `json:"source" yaml:"source"`
	Preloaded bool              `json:"preloaded" yaml:"preloaded"`
}

type Manifest struct {
	Enabled     bool     `json:"enabled"`
	Preload     bool     `json:"preload"`
	ConfigFiles []string `json:"config_files"`
	Servers     []Server `json:"servers"`
}

type fileConfig struct {
	MCPServers map[string]struct {
		Command string            `json:"command" yaml:"command"`
		Args    []string          `json:"args" yaml:"args"`
		Env     map[string]string `json:"env" yaml:"env"`
	} `json:"mcpServers" yaml:"mcpServers"`
	Servers []Server `json:"servers" yaml:"servers"`
}

func Load(cfg cfconfig.MCPConfig) (Manifest, error) {
	manifest := Manifest{Enabled: cfg.Enabled, Preload: cfg.Preload, ConfigFiles: cfg.ConfigFiles}
	if !cfg.Enabled {
		return manifest, nil
	}
	for _, server := range cfg.Servers {
		enabled := server.Enabled
		if !enabled {
			enabled = true
		}
		manifest.Servers = append(manifest.Servers, Server{
			Name:      server.Name,
			Command:   server.Command,
			Args:      server.Args,
			Env:       server.Env,
			Enabled:   enabled,
			Source:    "project-config",
			Preloaded: cfg.Preload && enabled,
		})
	}
	for _, path := range cfg.ConfigFiles {
		loaded, err := loadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return manifest, err
		}
		for _, server := range loaded {
			server.Source = path
			server.Preloaded = cfg.Preload && server.Enabled
			manifest.Servers = append(manifest.Servers, server)
		}
	}
	sort.Slice(manifest.Servers, func(i, j int) bool {
		return manifest.Servers[i].Name < manifest.Servers[j].Name
	})
	return manifest, nil
}

func PreloadText(manifest Manifest) string {
	if !manifest.Enabled || !manifest.Preload {
		return ""
	}
	var b strings.Builder
	for _, server := range manifest.Servers {
		if !server.Preloaded {
			continue
		}
		b.WriteString("- ")
		b.WriteString(server.Name)
		b.WriteString(": ")
		b.WriteString(server.Command)
		if len(server.Args) > 0 {
			b.WriteString(" ")
			b.WriteString(strings.Join(server.Args, " "))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func loadFile(path string) ([]Server, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg fileConfig
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	} else if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	var out []Server
	for name, server := range cfg.MCPServers {
		out = append(out, Server{
			Name:    name,
			Command: server.Command,
			Args:    server.Args,
			Env:     server.Env,
			Enabled: true,
		})
	}
	for _, server := range cfg.Servers {
		if server.Name == "" {
			continue
		}
		if !server.Enabled {
			server.Enabled = true
		}
		out = append(out, server)
	}
	return out, nil
}
