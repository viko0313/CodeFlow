package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/viko0313/CodeFlow/internal/config"
	"github.com/viko0313/CodeFlow/internal/memory"
	"github.com/viko0313/CodeFlow/internal/tools"
)

type fakeToolModel struct {
	calls int
}

func (m *fakeToolModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	m.calls++
	if m.calls == 1 {
		msg := schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "calculator",
				Arguments: `{"expression":"2+2"}`,
			},
		}})
		msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13}}
		return msg, nil
	}
	msg := schema.AssistantMessage("final answer", nil)
	msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 8, CompletionTokens: 4, TotalTokens: 12}}
	return msg, nil
}

func (m *fakeToolModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *fakeToolModel) BindTools(tools []*schema.ToolInfo) error {
	return nil
}

func (m *fakeToolModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

func TestRunWithToolsExecutesToolAndStoresTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Workspace = dir
	cfg.MaxActions = 3
	agent := &CodeFlowAgent{
		name:        "CodeFlow",
		instruction: getDefaultInstruction(),
		chatModel:   &fakeToolModel{},
		mm:          memory.NewMemoryManager(filepath.Join(dir, "memory")),
		cfg:         &cfg,
		toolList:    []tool.BaseTool{tools.NewCalculatorTool()},
	}

	resp, toolResults, err := agent.RunWithTools(context.Background(), "calculate", "thread")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "final answer" {
		t.Fatalf("unexpected response: %q", resp)
	}
	if len(toolResults) != 1 || !strings.Contains(toolResults[0], "calculator") {
		t.Fatalf("expected calculator tool result, got %+v", toolResults)
	}
	turns, err := agent.mm.GetRecentTurns(context.Background(), "thread", 1)
	if err != nil || len(turns) != 1 {
		t.Fatalf("expected stored turn: len=%d err=%v", len(turns), err)
	}
}

func TestRunDetailedAccumulatesRuntimeStats(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Workspace = dir
	cfg.MaxActions = 3
	agent := &CodeFlowAgent{
		name:        "CodeFlow",
		instruction: getDefaultInstruction(),
		chatModel:   &fakeToolModel{},
		mm:          memory.NewMemoryManager(filepath.Join(dir, "memory")),
		cfg:         &cfg,
		toolList:    []tool.BaseTool{tools.NewCalculatorTool()},
	}

	result, err := agent.RunDetailed(context.Background(), "calculate", "thread")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stats.TokenUsage.Known {
		t.Fatal("expected known token usage")
	}
	if result.Stats.TokenUsage.Prompt != 18 || result.Stats.TokenUsage.Completion != 7 || result.Stats.TokenUsage.Total != 25 {
		t.Fatalf("unexpected token usage: %+v", result.Stats.TokenUsage)
	}
	if !strings.Contains(result.Stats.FlowSummary(), "Tools") {
		t.Fatalf("expected tools in flow: %s", result.Stats.FlowSummary())
	}
}
