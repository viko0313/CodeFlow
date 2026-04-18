package main

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/cyberclaw/internal/agent"
)

func TestRenderRuntimePanelWithTokens(t *testing.T) {
	panel := renderRuntimePanel(agent.RuntimeStats{
		Provider:      "qwen",
		Model:         "qwen3.5-flash",
		TokenUsage:    agent.TokenUsage{Prompt: 1234, Completion: 128, Total: 1362, Known: true},
		ModelDuration: 1800 * time.Millisecond,
		TotalDuration: 2100 * time.Millisecond,
		Flow:          []string{"Input", "Memory", "Model", "Tools", "Model", "Output"},
		Tools:         []string{"get_system_model_info ok"},
		RecentTurns:   3,
		ProfileLoaded: true,
	})
	for _, want := range []string{"Runtime", "qwen/qwen3.5-flash", "prompt=1234", "Tools", "recent_turns=3"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("panel missing %q:\n%s", want, panel)
		}
	}
}

func TestRenderRuntimePanelUnknownTokens(t *testing.T) {
	panel := renderRuntimePanel(agent.RuntimeStats{Provider: "ollama", Model: "qwen3.5:4b"})
	if !strings.Contains(panel, "unknown") {
		t.Fatalf("expected unknown token display:\n%s", panel)
	}
}
