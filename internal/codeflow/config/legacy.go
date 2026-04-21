package config

import legacy "github.com/viko0313/CodeFlow/internal/config"

func (c *Config) ToLegacy() *legacy.Config {
	return &legacy.Config{
		Provider:    c.Provider,
		Model:       c.Model,
		APIKey:      c.APIKey,
		BaseURL:     c.BaseURL,
		Temperature: c.Temperature,
		Workspace:   c.DataDir,
		MaxMemory:   100,
		MaxTurns:    c.Runtime.MaxTurns,
		MaxActions:  c.Runtime.MaxActions,
	}
}
