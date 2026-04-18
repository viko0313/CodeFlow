package model

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"

	"github.com/cloudwego/cyberclaw/internal/config"
)

type ProviderManager struct{}

func NewProviderManager() *ProviderManager {
	return &ProviderManager{}
}

var baseURLs = map[string]string{
	"aliyun":     "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"dashscope":  "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"qwen":       "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"ollama":     "http://localhost:11434",
	"z.ai":       "https://open.bigmodel.cn/api/paas/v4",
	"tencent":    "https://api.hunyuan.cloud.tencent.com/v1",
	"volcengine": "https://ark.cn-beijing.volces.com/api/v3",
	"ark":        "https://ark.cn-beijing.volces.com/api/v3",
}

func (pm *ProviderManager) CreateChatModel(ctx context.Context, cfg *config.Config) (model.ChatModel, error) {
	providerName := normalizeProvider(cfg.Provider)

	switch providerName {
	case "openai", "aliyun", "dashscope", "qwen", "z.ai", "tencent", "volcengine", "ark":
		return pm.createOpenAICompatible(ctx, providerName, cfg)
	case "anthropic":
		return nil, fmt.Errorf("anthropic not implemented yet")
	case "ollama":
		return pm.createOllama(ctx, cfg)
	default:
		return pm.createOpenAICompatible(ctx, "openai", cfg)
	}
}

func (pm *ProviderManager) createOpenAICompatible(ctx context.Context, providerName string, cfg *config.Config) (model.ChatModel, error) {
	apiKey := resolveAPIKey(providerName, cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("API key not found for provider %q; set ARK_API_KEY, VOLCENGINE_API_KEY, OPENAI_API_KEY, or CYBERCLAW_API_KEY", providerName)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_API_BASE")
	}
	if baseURL == "" {
		baseURL = baseURLs[providerName]
	}
	temperature := float32(cfg.Temperature)

	switch providerName {
	case "ark", "volcengine":
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			Model:       cfg.Model,
			APIKey:      apiKey,
			BaseURL:     baseURL,
			Temperature: &temperature,
		})
	case "qwen", "dashscope", "aliyun":
		return qwen.NewChatModel(ctx, &qwen.ChatModelConfig{
			Model:       cfg.Model,
			APIKey:      apiKey,
			BaseURL:     baseURL,
			Temperature: &temperature,
		})
	default:
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			Model:       cfg.Model,
			APIKey:      apiKey,
			BaseURL:     baseURL,
			Temperature: &temperature,
		})
	}
}

func (pm *ProviderManager) createOllama(ctx context.Context, cfg *config.Config) (model.ChatModel, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("OLLAMA_BASE_URL")
	}
	if baseURL == "" {
		baseURL = baseURLs["ollama"]
	}
	temperature := float32(cfg.Temperature)
	return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   cfg.Model,
		Options: &ollama.Options{
			Temperature: temperature,
		},
	})
}

func normalizeProvider(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gpt", "openai":
		return "openai"
	case "claude", "anthropic":
		return "anthropic"
	case "volcano", "volc", "volcengine", "ark":
		return "ark"
	case "qwen", "dashscope", "aliyun":
		return "qwen"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func resolveAPIKey(provider, configured string) string {
	if configured != "" {
		return configured
	}
	envs := []string{"CYBERCLAW_API_KEY"}
	if provider == "ark" || provider == "volcengine" {
		envs = append(envs, "ARK_API_KEY", "VOLCENGINE_API_KEY")
	}
	if provider == "qwen" || provider == "dashscope" || provider == "aliyun" {
		envs = append(envs, "QWEN_API_KEY", "DASHSCOPE_API_KEY")
	}
	envs = append(envs, "OPENAI_API_KEY")
	for _, name := range envs {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}
