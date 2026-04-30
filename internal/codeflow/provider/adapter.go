package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	legacyconfig "github.com/viko0313/CodeFlow/internal/config"
	legacyprovider "github.com/viko0313/CodeFlow/internal/model"
)

type ToolSchemaFormat string

const (
	ToolSchemaFormatOpenAI ToolSchemaFormat = "openai"
	ToolSchemaFormatEino   ToolSchemaFormat = "eino"
)

type MessageFormat string

const (
	MessageFormatOpenAICompatible MessageFormat = "openai-compatible"
	MessageFormatAnthropic        MessageFormat = "anthropic"
	MessageFormatOllama           MessageFormat = "ollama"
)

type ProviderConfig struct {
	ProviderName string
	ModelName    string
	BaseURL      string
	APIKeyEnv    string
	Temperature  float64
	MaxTokens    int
	Timeout      time.Duration
}

type ModelCapability struct {
	SupportsToolCall         bool
	SupportsStreaming        bool
	SupportsVision           bool
	SupportsReasoning        bool
	SupportsParallelToolCall bool
	SupportsPromptCache      bool
	MaxContextTokens         int
	MaxOutputTokens          int
	ToolSchemaFormat         ToolSchemaFormat
	MessageFormat            MessageFormat
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type ProviderAdapter interface {
	Name() string
	BuildChatModel(ctx context.Context) (einomodel.ChatModel, error)
	NormalizeMessages(messages []*schema.Message) []*schema.Message
	BindTools(model einomodel.ChatModel, tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error)
	Stream(ctx context.Context, model any, messages []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error)
	Generate(ctx context.Context, model any, messages []*schema.Message, opts ...einomodel.Option) (*schema.Message, error)
	ParseToolCalls(msg *schema.Message) []schema.ToolCall
	NormalizeUsage(msg *schema.Message) Usage
	Capability() ModelCapability
	Config() ProviderConfig
}

type DefaultAdapter struct {
	name       string
	config     ProviderConfig
	legacy     *legacyconfig.Config
	capability ModelCapability
}

func NewAdapter(cfg *cfconfig.Config) (ProviderAdapter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	legacy := cfg.ToLegacy()
	name := NormalizeProviderName(legacy.Provider)
	if name == "" {
		name = "openai"
	}
	adapter := &DefaultAdapter{
		name:       name,
		config:     providerConfigFromLegacy(name, legacy),
		legacy:     legacy,
		capability: capabilityFor(name),
	}
	if name == "anthropic" {
		return adapter, nil
	}
	if !adapter.isSupported() {
		return nil, fmt.Errorf("provider %q is not supported", strings.TrimSpace(legacy.Provider))
	}
	return adapter, nil
}

func (a *DefaultAdapter) Name() string                { return a.name }
func (a *DefaultAdapter) Config() ProviderConfig      { return a.config }
func (a *DefaultAdapter) Capability() ModelCapability { return a.capability }

func (a *DefaultAdapter) BuildChatModel(ctx context.Context) (einomodel.ChatModel, error) {
	if a.name == "anthropic" {
		return nil, fmt.Errorf("provider %q is declared but not implemented yet", a.name)
	}
	if !a.isSupported() {
		return nil, fmt.Errorf("provider %q is not supported", a.name)
	}
	return legacyprovider.NewProviderManager().CreateChatModel(ctx, a.legacy)
}

func (a *DefaultAdapter) NormalizeMessages(messages []*schema.Message) []*schema.Message {
	return messages
}

func (a *DefaultAdapter) BindTools(model einomodel.ChatModel, tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	toolModel, ok := model.(einomodel.ToolCallingChatModel)
	if !ok {
		return nil, fmt.Errorf("provider %q model does not support tool calling", a.name)
	}
	return toolModel.WithTools(tools)
}

func (a *DefaultAdapter) Stream(ctx context.Context, model any, messages []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	streamer, ok := model.(interface {
		Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error)
	})
	if !ok {
		return nil, fmt.Errorf("provider %q model does not support streaming", a.name)
	}
	return streamer.Stream(ctx, a.NormalizeMessages(messages), opts...)
}

func (a *DefaultAdapter) Generate(ctx context.Context, model any, messages []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	generator, ok := model.(interface {
		Generate(context.Context, []*schema.Message, ...einomodel.Option) (*schema.Message, error)
	})
	if !ok {
		return nil, fmt.Errorf("provider %q model does not support generate", a.name)
	}
	return generator.Generate(ctx, a.NormalizeMessages(messages), opts...)
}

func (a *DefaultAdapter) ParseToolCalls(msg *schema.Message) []schema.ToolCall {
	if msg == nil {
		return nil
	}
	return msg.ToolCalls
}

func (a *DefaultAdapter) NormalizeUsage(msg *schema.Message) Usage {
	_ = msg
	return Usage{}
}

func (a *DefaultAdapter) isSupported() bool {
	switch a.name {
	case "openai", "qwen", "ark", "ollama", "anthropic":
		return true
	default:
		return false
	}
}

func NormalizeProviderName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "gpt", "openai", "openai-compatible", "z.ai", "tencent":
		return "openai"
	case "claude", "anthropic":
		return "anthropic"
	case "volcano", "volc", "volcengine", "ark":
		return "ark"
	case "qwen", "dashscope", "aliyun":
		return "qwen"
	case "ollama":
		return "ollama"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func capabilityFor(name string) ModelCapability {
	switch name {
	case "ark":
		return ModelCapability{SupportsToolCall: true, SupportsStreaming: true, SupportsVision: false, SupportsReasoning: true, SupportsParallelToolCall: true, SupportsPromptCache: false, MaxContextTokens: 128000, MaxOutputTokens: 8192, ToolSchemaFormat: ToolSchemaFormatOpenAI, MessageFormat: MessageFormatOpenAICompatible}
	case "qwen":
		return ModelCapability{SupportsToolCall: true, SupportsStreaming: true, SupportsVision: false, SupportsReasoning: true, SupportsParallelToolCall: true, SupportsPromptCache: false, MaxContextTokens: 128000, MaxOutputTokens: 8192, ToolSchemaFormat: ToolSchemaFormatOpenAI, MessageFormat: MessageFormatOpenAICompatible}
	case "ollama":
		return ModelCapability{SupportsToolCall: true, SupportsStreaming: true, SupportsVision: false, SupportsReasoning: false, SupportsParallelToolCall: false, SupportsPromptCache: false, MaxContextTokens: 32768, MaxOutputTokens: 4096, ToolSchemaFormat: ToolSchemaFormatEino, MessageFormat: MessageFormatOllama}
	case "anthropic":
		return ModelCapability{SupportsToolCall: false, SupportsStreaming: false, SupportsVision: true, SupportsReasoning: true, SupportsParallelToolCall: false, SupportsPromptCache: true, MaxContextTokens: 200000, MaxOutputTokens: 8192, ToolSchemaFormat: ToolSchemaFormatOpenAI, MessageFormat: MessageFormatAnthropic}
	default:
		return ModelCapability{SupportsToolCall: true, SupportsStreaming: true, SupportsVision: false, SupportsReasoning: true, SupportsParallelToolCall: true, SupportsPromptCache: false, MaxContextTokens: 128000, MaxOutputTokens: 8192, ToolSchemaFormat: ToolSchemaFormatOpenAI, MessageFormat: MessageFormatOpenAICompatible}
	}
}

func providerConfigFromLegacy(name string, cfg *legacyconfig.Config) ProviderConfig {
	apiEnv := "OPENAI_API_KEY"
	switch name {
	case "ark":
		apiEnv = "ARK_API_KEY"
	case "qwen":
		apiEnv = "QWEN_API_KEY"
	case "ollama":
		apiEnv = ""
	case "anthropic":
		apiEnv = "ANTHROPIC_API_KEY"
	}
	return ProviderConfig{
		ProviderName: name,
		ModelName:    strings.TrimSpace(cfg.Model),
		BaseURL:      strings.TrimSpace(cfg.BaseURL),
		APIKeyEnv:    apiEnv,
		Temperature:  cfg.Temperature,
	}
}
