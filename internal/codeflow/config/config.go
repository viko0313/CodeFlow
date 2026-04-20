package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Provider    string  `mapstructure:"provider" yaml:"provider"`
	Model       string  `mapstructure:"model" yaml:"model"`
	APIKey      string  `mapstructure:"api_key" yaml:"api_key"`
	BaseURL     string  `mapstructure:"base_url" yaml:"base_url"`
	Temperature float64 `mapstructure:"temperature" yaml:"temperature"`

	ProjectRoot string `mapstructure:"project_root" yaml:"project_root"`
	DataDir     string `mapstructure:"data_dir" yaml:"data_dir"`

	Storage     StorageConfig     `mapstructure:"storage" yaml:"storage"`
	Permissions PermissionConfig  `mapstructure:"permissions" yaml:"permissions"`
	Runtime     RuntimeConfig     `mapstructure:"runtime" yaml:"runtime"`
	Reserved    map[string]string `mapstructure:"reserved" yaml:"reserved,omitempty"`
}

type StorageConfig struct {
	PostgresDSN string `mapstructure:"postgres_dsn" yaml:"postgres_dsn"`
	RedisAddr   string `mapstructure:"redis_addr" yaml:"redis_addr"`
	RedisPass   string `mapstructure:"redis_password" yaml:"redis_password"`
	RedisDB     int    `mapstructure:"redis_db" yaml:"redis_db"`
}

type PermissionConfig struct {
	TrustedCommands []string `mapstructure:"trusted_commands" yaml:"trusted_commands"`
	TrustedDirs     []string `mapstructure:"trusted_dirs" yaml:"trusted_dirs"`
	WritableDirs    []string `mapstructure:"writable_dirs" yaml:"writable_dirs"`
}

type RuntimeConfig struct {
	MaxTurns   int `mapstructure:"max_turns" yaml:"max_turns"`
	MaxActions int `mapstructure:"max_actions" yaml:"max_actions"`
}

func Default(projectRoot string) Config {
	root := absOr(projectRoot, mustGetwd())
	return Config{
		Provider:    "ark",
		Model:       "doubao-seed-1-8-251228",
		APIKey:      "${ARK_API_KEY}",
		BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
		Temperature: 0,
		ProjectRoot: root,
		DataDir:     filepath.Join(root, ".codeflow"),
		Storage: StorageConfig{
			PostgresDSN: "${CODEFLOW_POSTGRES_DSN}",
			RedisAddr:   "localhost:6379",
			RedisDB:     0,
		},
		Runtime: RuntimeConfig{
			MaxTurns:   50,
			MaxActions: 20,
		},
		Reserved: map[string]string{
			"mcp":        "phase2",
			"milvus":     "phase2",
			"subagent":   "phase3",
			"checkpoint": "phase3",
			"evolution":  "phase4",
			"web":        "phase5",
		},
	}
}

func Load(projectRoot string) (*Config, error) {
	root := absOr(projectRoot, mustGetwd())
	loadDotEnv(filepath.Join(root, ".env"))
	loadDotEnv(".env")

	cfg := Default(root)
	if err := mergeFile(&cfg, filepath.Join(userConfigDir(), "config.yaml"), true); err != nil {
		return nil, err
	}
	projectConfig := filepath.Join(root, ".codeflow", "config.yaml")
	if err := rejectPlaintextSecrets(projectConfig); err != nil {
		return nil, err
	}
	if err := mergeFile(&cfg, projectConfig, true); err != nil {
		return nil, err
	}
	applyEnv(&cfg)
	cfg.expandEnv()
	cfg.applyProviderDefaults()
	cfg.ProjectRoot = root
	cfg.DataDir = absOr(cfg.DataDir, filepath.Join(root, ".codeflow"))
	if cfg.Runtime.MaxTurns <= 0 {
		cfg.Runtime.MaxTurns = 50
	}
	if cfg.Runtime.MaxActions <= 0 {
		cfg.Runtime.MaxActions = 20
	}
	return &cfg, nil
}

func EnsureProjectConfig(projectRoot string) error {
	cfg := Default(projectRoot)
	path := filepath.Join(cfg.ProjectRoot, ".codeflow", "config.yaml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := strings.Join([]string{
		"provider: \"ark\"",
		"model: \"doubao-seed-1-8-251228\"",
		"api_key: ${ARK_API_KEY}",
		"base_url: \"https://ark.cn-beijing.volces.com/api/v3\"",
		"temperature: 0.00",
		"storage:",
		"  postgres_dsn: ${CODEFLOW_POSTGRES_DSN}",
		"  redis_addr: \"localhost:6379\"",
		"  redis_password: ${CODEFLOW_REDIS_PASSWORD}",
		"  redis_db: 0",
		"permissions:",
		"  trusted_commands: []",
		"  trusted_dirs: []",
		"  writable_dirs: []",
		"runtime:",
		"  max_turns: 50",
		"  max_actions: 20",
		"",
	}, "\n")
	return os.WriteFile(path, []byte(content), 0600)
}

func Get(projectRoot, key string) (string, error) {
	cfg, err := Load(projectRoot)
	if err != nil {
		return "", err
	}
	switch key {
	case "provider":
		return cfg.Provider, nil
	case "model":
		return cfg.Model, nil
	case "base_url":
		return cfg.BaseURL, nil
	case "storage.postgres_dsn":
		return redact(cfg.Storage.PostgresDSN), nil
	case "storage.redis_addr":
		return cfg.Storage.RedisAddr, nil
	case "storage.redis_db":
		return strconv.Itoa(cfg.Storage.RedisDB), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func Set(projectRoot, key, value string) error {
	root := absOr(projectRoot, mustGetwd())
	if err := EnsureProjectConfig(root); err != nil {
		return err
	}
	path := filepath.Join(root, ".codeflow", "config.yaml")
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	if key == "api_key" && !isEnvRef(value) {
		return fmt.Errorf("api_key must be an environment reference like ${ARK_API_KEY}")
	}
	v.Set(key, value)
	return v.WriteConfigAs(path)
}

func mergeFile(cfg *Config, path string, optional bool) error {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		if optional || os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := v.Unmarshal(cfg); err != nil {
		return err
	}
	return nil
}

func rejectPlaintextSecrets(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`(?mi)^\s*(api_key|redis_password|postgres_dsn)\s*:\s*["']?([^"'\s#]+)`)
	for _, match := range re.FindAllStringSubmatch(string(data), -1) {
		if len(match) >= 3 && match[2] != "" && !isEnvRef(match[2]) {
			return fmt.Errorf("%s in %s must use an environment reference, not plaintext", match[1], path)
		}
	}
	return nil
}

func applyEnv(c *Config) {
	setString(&c.Provider, "CODEFLOW_PROVIDER", "CODEFLOW_PROVIDER")
	setString(&c.Model, "CODEFLOW_MODEL", "CODEFLOW_MODEL", "ARK_MODEL_ID")
	setString(&c.APIKey, "CODEFLOW_API_KEY", "CODEFLOW_API_KEY", "ARK_API_KEY", "QWEN_API_KEY", "OPENAI_API_KEY")
	setString(&c.BaseURL, "CODEFLOW_BASE_URL", "CODEFLOW_BASE_URL", "ARK_BASE_URL", "OPENAI_API_BASE")
	setString(&c.Storage.PostgresDSN, "CODEFLOW_POSTGRES_DSN")
	setString(&c.Storage.RedisAddr, "CODEFLOW_REDIS_ADDR")
	setString(&c.Storage.RedisPass, "CODEFLOW_REDIS_PASSWORD")
	if value := strings.TrimSpace(os.Getenv("CODEFLOW_REDIS_DB")); value != "" {
		if db, err := strconv.Atoi(value); err == nil {
			c.Storage.RedisDB = db
		}
	}
}

func setString(target *string, names ...string) {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			*target = value
			return
		}
	}
}

func (c *Config) expandEnv() {
	c.APIKey = expandOrEmpty(c.APIKey)
	c.BaseURL = os.ExpandEnv(c.BaseURL)
	c.Storage.PostgresDSN = expandOrEmpty(c.Storage.PostgresDSN)
	c.Storage.RedisAddr = os.ExpandEnv(c.Storage.RedisAddr)
	c.Storage.RedisPass = expandOrEmpty(c.Storage.RedisPass)
}

func (c *Config) applyProviderDefaults() {
	switch strings.ToLower(strings.TrimSpace(c.Provider)) {
	case "ollama":
		if c.Model == "" || c.Model == "doubao-seed-1-8-251228" {
			c.Model = "qwen3.5:4b"
		}
		if c.BaseURL == "" || strings.Contains(c.BaseURL, "volces.com") {
			c.BaseURL = "http://localhost:11434"
		}
	case "qwen", "dashscope", "aliyun":
		if c.BaseURL == "" || strings.Contains(c.BaseURL, "volces.com") {
			c.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
	case "ark", "volcengine":
		if c.BaseURL == "" {
			c.BaseURL = "https://ark.cn-beijing.volces.com/api/v3"
		}
	}
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

func isEnvRef(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "${") && strings.HasSuffix(strings.TrimSpace(value), "}")
}

func expandOrEmpty(value string) string {
	value = strings.TrimSpace(value)
	if isEnvRef(value) {
		expanded := os.ExpandEnv(value)
		if expanded == value {
			return ""
		}
		return expanded
	}
	return os.ExpandEnv(value)
}

func userConfigDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codeflow")
	}
	return ".codeflow"
}

func absOr(path, fallback string) string {
	if strings.TrimSpace(path) == "" {
		path = fallback
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 10 {
		return "***"
	}
	return value[:6] + "***" + value[len(value)-4:]
}
