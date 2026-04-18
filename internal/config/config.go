package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Provider    string  `mapstructure:"provider"`
	Model       string  `mapstructure:"model"`
	APIKey      string  `mapstructure:"api_key"`
	BaseURL     string  `mapstructure:"base_url"`
	Temperature float64 `mapstructure:"temperature"`

	Workspace  string `mapstructure:"workspace"`
	MaxMemory  int    `mapstructure:"max_memory"`
	MaxTurns   int    `mapstructure:"max_turns"`
	MaxActions int    `mapstructure:"max_actions"`
}

var GlobalConfig *Config

func InitConfig(configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	GlobalConfig = cfg
	return nil
}

func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()
	loadDotEnv(".env")
	if err := mergeConfigFile(&cfg, configPath, false); err != nil {
		return nil, err
	}
	if configPath == "" {
		if err := mergeConfigFile(&cfg, "config.local.yaml", true); err != nil {
			return nil, err
		}
	}
	applyEnvOverrides(&cfg)
	cfg.expandEnv()
	cfg.applyDefaults()
	cfg.adjustPaths()
	return &cfg, nil
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func DefaultConfig() Config {
	return Config{
		Provider:    "ark",
		Model:       "doubao-seed-1-8-251228",
		BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
		Temperature: 0,
		Workspace:   "./workspace",
		MaxMemory:   100,
		MaxTurns:    50,
		MaxActions:  20,
	}
}

func mergeConfigFile(cfg *Config, path string, optional bool) error {
	v := viper.New()
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("$HOME/.cyberclaw")
	}
	if err := v.ReadInConfig(); err != nil {
		if optional || isConfigNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to read config: %w", err)
	}
	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return nil
}

func isConfigNotFound(err error) bool {
	_, ok := err.(viper.ConfigFileNotFoundError)
	return ok || os.IsNotExist(err)
}

func applyEnvOverrides(c *Config) {
	setStringFromEnv(&c.Provider, "CYBERCLAW_PROVIDER", "DEFAULT_PROVIDER")
	setStringFromEnv(&c.Model, "CYBERCLAW_MODEL", "DEFAULT_MODEL", "ARK_MODEL_ID")
	setStringFromEnv(&c.APIKey, "CYBERCLAW_API_KEY", "ARK_API_KEY", "VOLCENGINE_API_KEY", "QWEN_API_KEY", "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	setStringFromEnv(&c.BaseURL, "CYBERCLAW_BASE_URL", "ARK_BASE_URL", "VOLCENGINE_BASE_URL", "OLLAMA_BASE_URL", "OPENAI_API_BASE")
	setStringFromEnv(&c.Workspace, "CYBERCLAW_WORKSPACE")
}

func setStringFromEnv(target *string, names ...string) {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			*target = value
			return
		}
	}
}

func (c *Config) expandEnv() {
	c.Provider = os.ExpandEnv(c.Provider)
	c.Model = os.ExpandEnv(c.Model)
	c.APIKey = os.ExpandEnv(c.APIKey)
	c.BaseURL = os.ExpandEnv(c.BaseURL)
	c.Workspace = os.ExpandEnv(c.Workspace)
	if strings.HasPrefix(c.APIKey, "${") && strings.HasSuffix(c.APIKey, "}") {
		c.APIKey = ""
	}
}

func (c *Config) applyDefaults() {
	defaults := DefaultConfig()
	if c.Provider == "" {
		c.Provider = defaults.Provider
	}
	if c.Model == "" {
		c.Model = defaults.Model
	}
	if strings.EqualFold(c.Provider, "ollama") && (c.Model == defaults.Model || c.Model == "") {
		c.Model = "qwen3.5:4b"
	}
	if (c.BaseURL == "" || c.BaseURL == defaults.BaseURL) && (strings.EqualFold(c.Provider, "ark") || strings.EqualFold(c.Provider, "volcengine")) {
		c.BaseURL = defaults.BaseURL
	}
	if (c.BaseURL == "" || c.BaseURL == defaults.BaseURL) && (strings.EqualFold(c.Provider, "qwen") || strings.EqualFold(c.Provider, "dashscope") || strings.EqualFold(c.Provider, "aliyun")) {
		c.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if (c.BaseURL == "" || c.BaseURL == defaults.BaseURL) && strings.EqualFold(c.Provider, "ollama") {
		c.BaseURL = "http://localhost:11434"
	}
	if c.Workspace == "" {
		c.Workspace = defaults.Workspace
	}
	if c.MaxMemory <= 0 {
		c.MaxMemory = defaults.MaxMemory
	}
	if c.MaxTurns <= 0 {
		c.MaxTurns = defaults.MaxTurns
	}
	if c.MaxActions <= 0 {
		c.MaxActions = defaults.MaxActions
	}
}

func (c *Config) adjustPaths() {
	if c.Workspace == "" {
		execPath, _ := os.Executable()
		c.Workspace = filepath.Join(filepath.Dir(execPath), "workspace")
	}
	if abs, err := filepath.Abs(c.Workspace); err == nil {
		c.Workspace = abs
	}

	dirs := []string{
		c.Workspace,
		c.MemoryDir(),
		c.PersonasDir(),
		c.ScriptsDir(),
		c.OfficeDir(),
		c.SkillsDir(),
	}

	for _, d := range dirs {
		_ = os.MkdirAll(d, 0755)
	}
}

func WriteLocalConfig(path string, cfg Config) error {
	if path == "" {
		path = "config.local.yaml"
	}
	apiEnv := apiKeyEnvName(cfg.Provider)
	apiLine := "api_key: \"\"\n"
	if apiEnv != "" {
		apiLine = fmt.Sprintf("api_key: ${%s}\n", apiEnv)
	}
	content := fmt.Sprintf("provider: %q\nmodel: %q\n%sbase_url: %q\ntemperature: %.2f\nworkspace: %q\nmax_memory: %d\nmax_turns: %d\nmax_actions: %d\n",
		cfg.Provider, cfg.Model, apiLine, cfg.BaseURL, cfg.Temperature, cfg.Workspace, cfg.MaxMemory, cfg.MaxTurns, cfg.MaxActions)
	return os.WriteFile(path, []byte(content), 0600)
}

func apiKeyEnvName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qwen", "dashscope", "aliyun":
		return "QWEN_API_KEY"
	case "ark", "volcengine":
		return "ARK_API_KEY"
	case "ollama":
		return ""
	default:
		return "OPENAI_API_KEY"
	}
}

func (c *Config) MemoryDir() string   { return filepath.Join(c.Workspace, "memory") }
func (c *Config) PersonasDir() string { return filepath.Join(c.Workspace, "personas") }
func (c *Config) ScriptsDir() string  { return filepath.Join(c.Workspace, "scripts") }
func (c *Config) OfficeDir() string   { return filepath.Join(c.Workspace, "office") }
func (c *Config) SkillsDir() string   { return filepath.Join(c.OfficeDir(), "skills") }
func (c *Config) TasksFile() string   { return filepath.Join(c.Workspace, "tasks.json") }
func (c *Config) DBPath() string      { return filepath.Join(c.Workspace, "state.db") }
