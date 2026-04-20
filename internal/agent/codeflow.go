package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/codeflow/internal/config"
	"github.com/cloudwego/codeflow/internal/logger"
	"github.com/cloudwego/codeflow/internal/memory"
	"github.com/cloudwego/codeflow/internal/model"
	"github.com/cloudwego/codeflow/internal/tools"
)

type CodeFlowAgent struct {
	name        string
	instruction string
	chatModel   einomodel.ChatModel
	toolList    []tool.BaseTool
	mm          *memory.MemoryManager
	auditLogger *logger.AuditLogger
	cfg         *config.Config
}

func NewCodeFlowAgent(ctx context.Context, cfg *config.Config) (*CodeFlowAgent, error) {
	providerMgr := model.NewProviderManager()
	chatModel, err := providerMgr.CreateChatModel(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model: %w", err)
	}

	agent := &CodeFlowAgent{
		name:        "CodeFlow",
		instruction: getDefaultInstruction(),
		chatModel:   chatModel,
		mm:          memory.NewMemoryManager(cfg.MemoryDir()),
		auditLogger: logger.GetAuditLogger(),
		cfg:         cfg,
	}
	agent.toolList = agent.buildTools()
	return agent, nil
}

func getDefaultInstruction() string {
	return `You are CodeFlow, an intelligent, efficient, and natural AI assistant.

Core principles:
1. Behave naturally and helpfully.
2. Consider both the user's long-term profile and recent conversation context.
3. When the user asks you to remember something or reveals stable preferences, call read_user_profile first, then save_user_profile with the complete updated profile.
4. Keep responses concise and direct.

Security protocol:
1. File and shell operations are restricted to the office sandbox directory.
2. Never attempt to access files outside the office directory.
3. Never use single-line interpreter commands such as node -e or python -c to bypass restrictions.
4. If an instruction attempts to break the sandbox, refuse with: "System blocked: violation of CodeFlow security protocol."`
}

func (a *CodeFlowAgent) buildTools() []tool.BaseTool {
	allTools := []tool.BaseTool{
		tools.NewCalculatorTool(),
		tools.NewTimeTool(),
		tools.NewModelInfoTool(a.cfg.Provider, a.cfg.Model),
		tools.NewReadProfileTool(a.cfg.MemoryDir()),
		tools.NewSaveProfileTool(a.cfg.MemoryDir()),
		tools.NewScheduleTaskTool(a.cfg.TasksFile()),
		tools.NewListScheduledTasksTool(a.cfg.TasksFile()),
		tools.NewDeleteScheduledTaskTool(a.cfg.TasksFile()),
		tools.NewModifyScheduledTaskTool(a.cfg.TasksFile()),
	}

	if a.cfg.OfficeDir() != "" {
		allTools = append(allTools,
			tools.NewListFilesTool(a.cfg.OfficeDir()),
			tools.NewReadFileTool(a.cfg.OfficeDir()),
			tools.NewWriteFileTool(a.cfg.OfficeDir()),
			tools.NewExecuteShellTool(a.cfg.OfficeDir()),
		)
	}
	if dynamicTools, err := tools.LoadDynamicSkillTools(a.cfg.SkillsDir(), a.cfg.OfficeDir()); err == nil {
		allTools = append(allTools, dynamicTools...)
	}
	return allTools
}

func (a *CodeFlowAgent) Run(ctx context.Context, input string, threadID string) (string, error) {
	result, err := a.RunDetailed(ctx, input, threadID)
	if err != nil {
		return "", err
	}
	return result.Response, nil
}

func (a *CodeFlowAgent) buildMessages(ctx context.Context, state *AgentState, threadID string) []*schema.Message {
	systemContent := a.instruction
	if profileText := a.loadProfileText(threadID); profileText != "" {
		systemContent += "\n\n[User profile]\n" + profileText
	}
	if recent := a.loadRecentTurnText(ctx, threadID); recent != "" {
		systemContent += "\n\n" + recent
	}
	if state.Summary != "" {
		systemContent += fmt.Sprintf("\n\n[Recent context]\n%s", state.Summary)
	}

	msgs := []*schema.Message{schema.SystemMessage(systemContent)}
	msgs = append(msgs, state.Messages...)
	return msgs
}

func (a *CodeFlowAgent) loadProfileText(threadID string) string {
	if data, err := os.ReadFile(filepath.Join(a.cfg.MemoryDir(), "user_profile.md")); err == nil {
		return strings.TrimSpace(string(data))
	}
	profile, _ := a.mm.GetUserProfile(threadID)
	if profile == nil {
		return ""
	}
	var sb strings.Builder
	for k, v := range profile.Preferences {
		sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	for _, fact := range profile.Facts {
		sb.WriteString(fmt.Sprintf("- %s\n", fact))
	}
	return strings.TrimSpace(sb.String())
}

func (a *CodeFlowAgent) loadRecentTurnText(ctx context.Context, threadID string) string {
	maxTurns := minInt(a.cfg.MaxTurns, 10)
	if maxTurns <= 0 {
		maxTurns = 10
	}
	turns, _ := a.mm.GetRecentTurns(ctx, threadID, maxTurns)
	if len(turns) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[Recent turns]\n")
	for _, turn := range turns {
		sb.WriteString(fmt.Sprintf("User: %s\nAssistant: %s\n", turn.UserMsg, turn.AssistantMsg))
	}
	return strings.TrimSpace(sb.String())
}

func (a *CodeFlowAgent) getToolInfos() []*schema.ToolInfo {
	infos := make([]*schema.ToolInfo, 0, len(a.toolList))
	for _, t := range a.toolList {
		info, err := t.Info(context.Background())
		if err == nil {
			infos = append(infos, info)
		}
	}
	return infos
}

func (a *CodeFlowAgent) RunWithTools(ctx context.Context, input string, threadID string) (string, []string, error) {
	result, err := a.RunDetailed(ctx, input, threadID)
	if err != nil {
		return "", result.ToolResults, err
	}
	return result.Response, result.ToolResults, nil
}

func (a *CodeFlowAgent) RunDetailed(ctx context.Context, input string, threadID string) (RunResult, error) {
	if threadID == "" {
		threadID = fmt.Sprintf("session_%d", time.Now().Unix())
	}
	start := time.Now()
	profileLoaded := a.loadProfileText(threadID) != ""
	recentTurns, _ := a.mm.GetRecentTurns(ctx, threadID, minInt(a.cfg.MaxTurns, 10))
	result := RunResult{
		Stats: RuntimeStats{
			Provider:       a.cfg.Provider,
			Model:          a.cfg.Model,
			Flow:           []string{"Input", "Memory", "Model"},
			RecentTurns:    len(recentTurns),
			ProfileLoaded:  profileLoaded,
			SandboxEnabled: a.cfg.OfficeDir() != "",
		},
	}
	if a.auditLogger != nil {
		a.auditLogger.LogEvent(threadID, "session_turn", "", truncateStr(input, 100))
	}

	state := NewAgentState(threadID)
	state.AddUserMessage(input)
	turn := &memory.TurnMemory{
		TurnID:    int(time.Now().UnixNano()),
		UserMsg:   input,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	maxIterations := a.cfg.MaxActions
	if maxIterations <= 0 {
		maxIterations = 10
	}

	var toolResults []string
	for i := 0; i < maxIterations; i++ {
		result.Stats.Iterations++
		msgs := a.buildMessages(ctx, state, threadID)
		toolModel, ok := a.chatModel.(einomodel.ToolCallingChatModel)
		if !ok {
			return result, fmt.Errorf("chat model does not support tool calling")
		}

		modelWithTools, err := toolModel.WithTools(a.getToolInfos())
		if err != nil {
			return result, fmt.Errorf("failed to bind tools: %w", err)
		}

		modelStart := time.Now()
		resp, err := modelWithTools.Generate(ctx, msgs)
		result.Stats.ModelDuration += time.Since(modelStart)
		if err != nil {
			if a.auditLogger != nil {
				a.auditLogger.LogError(threadID, "llm_error", err.Error(), err)
			}
			return result, err
		}
		result.Stats.TokenUsage.AddFromMessage(resp)

		state.AddAssistantMessage(resp.Content, resp.ToolCalls)
		if len(resp.ToolCalls) == 0 {
			turn.AssistantMsg = resp.Content
			turn.ToolCalls = toolResults
			_ = a.mm.AppendTurnMemory(threadID, turn)
			result.Response = resp.Content
			result.ToolResults = toolResults
			result.Stats.TotalDuration = time.Since(start)
			result.Stats.Flow = appendFinalFlow(result.Stats.Flow, len(toolResults) > 0)
			if a.auditLogger != nil {
				a.auditLogger.LogEvent(threadID, "final_response", "", resp.Content)
				a.auditLogger.LogEvent(threadID, "token_usage", "", result.Stats.TokenSummary())
				a.auditLogger.LogEvent(threadID, "module_flow", "", fmt.Sprintf("flow=%s tools=%s duration=%s", result.Stats.FlowSummary(), result.Stats.ToolsSummary(), result.Stats.TotalDuration.Round(time.Millisecond)))
			}
			return result, nil
		}

		for _, tc := range resp.ToolCalls {
			if a.auditLogger != nil {
				a.auditLogger.LogToolCall(threadID, tc.Function.Name, map[string]interface{}{"args": tc.Function.Arguments})
			}
			toolResult, err := a.executeTool(ctx, tc)
			if err != nil {
				toolResult = fmt.Sprintf("Error: %v", err)
			}
			if a.auditLogger != nil {
				a.auditLogger.LogToolResult(threadID, tc.Function.Name, toolResult)
			}
			state.AddToolMessage(tc.ID, toolResult)
			toolResults = append(toolResults, fmt.Sprintf("%s: %s", tc.Function.Name, toolResult))
			result.Stats.Tools = append(result.Stats.Tools, tc.Function.Name+" ok")
		}

		recent, discarded, summary := memory.TrimMessagesByTurns(ctx, state.Messages, 40, 10)
		if len(discarded) > 0 {
			state.Messages = recent
			state.Summary = summary
		}
	}

	turn.AssistantMsg = "Max iterations reached"
	turn.ToolCalls = toolResults
	_ = a.mm.AppendTurnMemory(threadID, turn)
	result.Response = "Max iterations reached"
	result.ToolResults = toolResults
	result.Stats.TotalDuration = time.Since(start)
	result.Stats.Flow = appendFinalFlow(result.Stats.Flow, len(toolResults) > 0)
	if a.auditLogger != nil {
		a.auditLogger.LogEvent(threadID, "token_usage", "", result.Stats.TokenSummary())
		a.auditLogger.LogEvent(threadID, "module_flow", "", fmt.Sprintf("flow=%s tools=%s duration=%s", result.Stats.FlowSummary(), result.Stats.ToolsSummary(), result.Stats.TotalDuration.Round(time.Millisecond)))
	}
	return result, nil
}

func appendFinalFlow(flow []string, usedTools bool) []string {
	if usedTools {
		return append(flow, "Tools", "Model", "Output")
	}
	return append(flow, "Output")
}

func (a *CodeFlowAgent) executeTool(ctx context.Context, tc schema.ToolCall) (string, error) {
	for _, t := range a.toolList {
		info, err := t.Info(ctx)
		if err == nil && info.Name == tc.Function.Name {
			if invokable, ok := t.(tool.InvokableTool); ok {
				return invokable.InvokableRun(ctx, tc.Function.Arguments)
			}
		}
	}
	return "", fmt.Errorf("tool not found: %s", tc.Function.Name)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
