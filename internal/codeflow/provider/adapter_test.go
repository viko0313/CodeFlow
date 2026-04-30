package provider

import (
	"context"
	"strings"
	"testing"

	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
)

func TestNormalizeProviderName(t *testing.T) {
	cases := map[string]string{
		"openai":     "openai",
		"gpt":        "openai",
		"ark":        "ark",
		"volcengine": "ark",
		"qwen":       "qwen",
		"dashscope":  "qwen",
		"ollama":     "ollama",
		"anthropic":  "anthropic",
	}
	for input, want := range cases {
		if got := NormalizeProviderName(input); got != want {
			t.Fatalf("NormalizeProviderName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNewAdapterCapabilityMatrix(t *testing.T) {
	tests := []struct {
		provider  string
		name      string
		toolCall  bool
		streaming bool
	}{
		{provider: "openai", name: "openai", toolCall: true, streaming: true},
		{provider: "dashscope", name: "qwen", toolCall: true, streaming: true},
		{provider: "ark", name: "ark", toolCall: true, streaming: true},
		{provider: "ollama", name: "ollama", toolCall: true, streaming: true},
		{provider: "anthropic", name: "anthropic", toolCall: false, streaming: false},
	}
	for _, tt := range tests {
		adapter, err := NewAdapter(&cfconfig.Config{Provider: tt.provider, Model: "test-model"})
		if err != nil {
			t.Fatalf("NewAdapter(%q) returned error: %v", tt.provider, err)
		}
		if adapter.Name() != tt.name {
			t.Fatalf("adapter name = %q, want %q", adapter.Name(), tt.name)
		}
		capability := adapter.Capability()
		if capability.SupportsToolCall != tt.toolCall {
			t.Fatalf("%s SupportsToolCall = %v, want %v", tt.provider, capability.SupportsToolCall, tt.toolCall)
		}
		if capability.SupportsStreaming != tt.streaming {
			t.Fatalf("%s SupportsStreaming = %v, want %v", tt.provider, capability.SupportsStreaming, tt.streaming)
		}
	}
}

func TestNewAdapterUnsupportedProvider(t *testing.T) {
	_, err := NewAdapter(&cfconfig.Config{Provider: "mystery", Model: "x"})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestAnthropicBuildChatModelReturnsClearError(t *testing.T) {
	adapter, err := NewAdapter(&cfconfig.Config{Provider: "anthropic", Model: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.BuildChatModel(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected not implemented error, got %v", err)
	}
}
