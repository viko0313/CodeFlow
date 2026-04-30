package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	cfmemory "github.com/viko0313/CodeFlow/internal/codeflow/memory"
	"github.com/viko0313/CodeFlow/internal/codeflow/observability"
	"github.com/viko0313/CodeFlow/internal/codeflow/plan"
	"github.com/viko0313/CodeFlow/internal/codeflow/provider"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
	cftools "github.com/viko0313/CodeFlow/internal/codeflow/tools"
)

type EventType string

const (
	EventStatus        EventType = "status"
	EventToken         EventType = "token"
	EventOutput        EventType = "output"
	EventError         EventType = "error"
	EventStats         EventType = "stats"
	EventToolStarted   EventType = "tool.started"
	EventToolCompleted EventType = "tool.completed"
	EventToolFailed    EventType = "tool.failed"
	EventIteration     EventType = "iteration"
)

type Event struct {
	Type           EventType
	Content        string
	ToolName       string
	ToolCallID     string
	Iteration      int
	DurationMillis int64
}

type Request struct {
	SessionID     string
	RequestID     string
	RunID         string
	WorkspaceID   string
	PlanID        string
	ProjectRoot   string
	Input         string
	AgentMD       string
	Context       string
	PlanEnabled   bool
	Toolset       string
	MaxIterations int
}

type Engine interface {
	Run(ctx context.Context, req Request) (<-chan Event, error)
}

type LLMEngine struct {
	cfg        *cfconfig.Config
	adapter    provider.ProviderAdapter
	model      einomodel.ChatModel
	memory     cfmemory.ShortTermMemory
	messages   storage.MessageStore
	summaries  storage.SummaryStore
	compressor *cfmemory.Compressor
	plans      *plan.Service
	traces     storage.TraceStore
	registry   *cftools.ToolRegistry
	executor   *cftools.Executor
	runs       *run.Recorder
	logger     *slog.Logger
}

type Option func(*LLMEngine)

type messageRole string

const (
	roleSystem    messageRole = "system"
	roleUser      messageRole = "user"
	roleAssistant messageRole = "assistant"
)

func WithMessageStore(store storage.MessageStore) Option {
	return func(e *LLMEngine) {
		e.messages = store
	}
}

func WithTraceStore(store storage.TraceStore) Option {
	return func(e *LLMEngine) {
		e.traces = store
	}
}

func WithToolExecutor(executor *cftools.Executor) Option {
	return func(e *LLMEngine) {
		e.executor = executor
	}
}

func WithToolRegistry(registry *cftools.ToolRegistry) Option {
	return func(e *LLMEngine) {
		e.registry = registry
	}
}

func WithRunRecorder(recorder *run.Recorder) Option {
	return func(e *LLMEngine) {
		e.runs = recorder
	}
}

func WithSummaryStore(store storage.SummaryStore) Option {
	return func(e *LLMEngine) {
		e.summaries = store
	}
}

func WithMemoryCompressor(compressor *cfmemory.Compressor) Option {
	return func(e *LLMEngine) {
		e.compressor = compressor
	}
}

func WithPlanService(service *plan.Service) Option {
	return func(e *LLMEngine) {
		e.plans = service
	}
}

func New(ctx context.Context, cfg *cfconfig.Config, memory cfmemory.ShortTermMemory, opts ...Option) (*LLMEngine, error) {
	adapter, err := provider.NewAdapter(cfg)
	if err != nil {
		return nil, err
	}
	chatModel, err := adapter.BuildChatModel(ctx)
	if err != nil {
		return nil, err
	}
	engine := &LLMEngine{
		cfg:      cfg,
		adapter:  adapter,
		model:    chatModel,
		memory:   memory,
		registry: cftools.DefaultRegistry(),
		logger:   observability.NewLogger("codeflow-engine"),
	}
	for _, opt := range opts {
		opt(engine)
	}
	return engine, nil
}

func (e *LLMEngine) Run(ctx context.Context, req Request) (<-chan Event, error) {
	out := make(chan Event, 16)
	ctx = e.prepareContext(ctx, req)
	go func() {
		defer close(out)
		start := time.Now()
		runRecord := run.AgentRun{
			ID:            strings.TrimSpace(req.RunID),
			SessionID:     req.SessionID,
			WorkspaceID:   req.WorkspaceID,
			PlanID:        req.PlanID,
			ModelProvider: e.cfg.Provider,
			ModelName:     e.cfg.Model,
		}
		if e.runs != nil && e.runs.Enabled() {
			if created, err := e.runs.Start(ctx, runRecord); err == nil && created != nil {
				runRecord = *created
				ctx = observability.WithRunID(ctx, runRecord.ID)
				req.RunID = runRecord.ID
				_ = e.runs.Event(ctx, run.RunEvent{RunID: runRecord.ID, Type: run.EventUserInput, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"input": req.Input}})
			}
		}
		e.logger.InfoContext(ctx, "model request started",
			slog.String("component", "engine"),
			slog.String("event", "model.request.started"),
			slog.String("request_id", observability.RequestIDFromContext(ctx)),
			slog.String("session_id", req.SessionID),
		)
		out <- Event{Type: EventStatus, Content: "thinking"}
		if e.runs != nil && e.runs.Enabled() && req.RunID != "" {
			_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventModelStart, Timestamp: time.Now().UTC(), RequestID: req.RequestID})
		}
		e.recordMessage(ctx, storage.MessageRecord{SessionID: req.SessionID, RequestID: req.RequestID, Role: string(roleUser), Content: req.Input})
		if e.shouldUseAgentLoop(ctx, req) {
			e.runAgentLoop(ctx, req, out, start)
			return
		}
		messages := e.buildMessages(ctx, req, nil)
		if e.adapter != nil && e.adapter.Capability().SupportsStreaming {
			if stream, err := e.adapter.Stream(ctx, e.model, messages); err == nil && stream != nil {
				e.consumeStream(ctx, stream, out, req, start)
				return
			}
		}
		resp, err := e.adapter.Generate(ctx, e.model, messages)
		if err != nil {
			e.logger.ErrorContext(ctx, "model request failed",
				slog.String("component", "engine"),
				slog.String("event", "model.request.failed"),
				slog.String("request_id", observability.RequestIDFromContext(ctx)),
				slog.String("session_id", req.SessionID),
				slog.String("error", err.Error()),
			)
			out <- Event{Type: EventError, Content: err.Error()}
			if e.runs != nil && req.RunID != "" {
				_, _ = e.runs.Finish(ctx, runRecord, run.StatusFailed, err.Error())
				_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventError, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"error": err.Error()}})
			}
			return
		}
		content := strings.TrimSpace(resp.Content)
		if req.PlanEnabled || e.cfg.Agent.PlanEnabled {
			if created, err := e.persistPlan(req, content); err == nil && created != nil {
				req.PlanID = created.ID
			}
		}
		e.recordMessage(ctx, storage.MessageRecord{SessionID: req.SessionID, RequestID: req.RequestID, Role: string(roleAssistant), Content: content})
		out <- Event{Type: EventOutput, Content: content}
		out <- Event{Type: EventStats, Content: fmt.Sprintf("duration=%s", time.Since(start).Round(time.Millisecond))}
		if e.runs != nil && req.RunID != "" {
			_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventModelEnd, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"output_chars": len(content)}})
			runRecord.TotalTokens = e.adapter.NormalizeUsage(resp).TotalTokens
			_, _ = e.runs.Finish(ctx, runRecord, run.StatusCompleted, "")
		}
		e.logger.InfoContext(ctx, "model request completed",
			slog.String("component", "engine"),
			slog.String("event", "model.request.completed"),
			slog.String("request_id", observability.RequestIDFromContext(ctx)),
			slog.String("session_id", req.SessionID),
			slog.Int("output_chars", len(content)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	}()
	return out, nil
}

func (e *LLMEngine) shouldUseAgentLoop(ctx context.Context, req Request) bool {
	if req.PlanEnabled || e.cfg.Agent.PlanEnabled {
		return false
	}
	if e.registry == nil {
		return false
	}
	if e.adapter == nil || !e.adapter.Capability().SupportsToolCall {
		return false
	}
	if _, ok := e.model.(einomodel.ToolCallingChatModel); !ok {
		return false
	}
	defs, err := e.registry.Definitions(ctx, req.Toolset)
	return err == nil && len(defs) > 0
}

func (e *LLMEngine) buildMessages(ctx context.Context, req Request, todos *cftools.TodoStore) []*schema.Message {
	system := "You are CodeFlow Agent, a terminal-native enterprise coding assistant. Prefer concise, auditable steps. Do not modify files or run commands directly; ask the host tool executor to handle privileged operations."
	system += "\n\n[Agent runtime]\nmode=" + e.cfg.Agent.Mode
	if req.PlanEnabled || e.cfg.Agent.PlanEnabled {
		system += "\nplan=true: respond with a concise implementation plan before execution-oriented guidance."
	} else {
		system += "\nplan=false: answer directly unless the user asks for planning."
	}
	if strings.TrimSpace(req.AgentMD) != "" {
		system += "\n\n[Project rules from AGENT.md]\n" + strings.TrimSpace(req.AgentMD)
	}
	if strings.TrimSpace(req.Context) != "" {
		system += "\n\n[Preloaded CodeFlow context]\n" + strings.TrimSpace(req.Context)
	}
	if todos != nil {
		if snapshot := todos.Snapshot(); snapshot != "" {
			system += "\n\n" + snapshot
		}
	}
	messages := []*schema.Message{schema.SystemMessage(system)}
	if e.summaries != nil && req.SessionID != "" {
		if summary, err := e.summaries.GetSessionSummary(req.SessionID); err == nil && summary != nil && strings.TrimSpace(summary.Summary) != "" {
			messages = append(messages, schema.SystemMessage("[Session summary]\n"+strings.TrimSpace(summary.Summary)))
		}
	}
	if e.messages != nil {
		limit := e.cfg.Runtime.MaxContextTurns
		if limit <= 0 {
			limit = 40
		}
		if stored, err := e.messages.ListMessages(ctx, req.SessionID, limit); err == nil && len(stored) > 0 {
			messages = append(messages, e.toSchemaMessages(stored)...)
			return e.compressMessagesIfNeeded(messages, req, todos)
		}
	}
	if e.memory != nil {
		if turns, err := e.memory.GetRecent(ctx, req.SessionID, 20); err == nil && len(turns) > 0 {
			for _, turn := range turns {
				switch strings.ToLower(strings.TrimSpace(turn.Role)) {
				case string(roleAssistant):
					messages = append(messages, schema.AssistantMessage(turn.Content, nil))
				case string(roleSystem):
					messages = append(messages, schema.SystemMessage(turn.Content))
				default:
					messages = append(messages, schema.UserMessage(turn.Content))
				}
			}
		}
	}
	messages = append(messages, schema.UserMessage(req.Input))
	return e.compressMessagesIfNeeded(messages, req, todos)
}

func (e *LLMEngine) runAgentLoop(ctx context.Context, req Request, out chan<- Event, start time.Time) {
	trace := newTurnTrace(e.traces, req)
	trace.record(ctx, storage.TraceEvent{EventType: "turn.started", Status: "ok", Payload: map[string]any{"input_chars": len(req.Input), "toolset": req.Toolset}})
	defs, err := e.registry.Definitions(ctx, req.Toolset)
	if err != nil {
		trace.record(ctx, storage.TraceEvent{EventType: "turn.failed", Status: "error", ErrorType: "tool_definitions_failed", Payload: map[string]any{"error": err.Error()}})
		out <- Event{Type: EventError, Content: err.Error()}
		return
	}
	if e.adapter == nil || !e.adapter.Capability().SupportsToolCall {
		trace.record(ctx, storage.TraceEvent{EventType: "turn.failed", Status: "error", ErrorType: "model_without_tool_calling"})
		out <- Event{Type: EventError, Content: "chat model does not support tool calling"}
		return
	}
	modelWithTools, err := e.adapter.BindTools(e.model, defs)
	if err != nil {
		trace.record(ctx, storage.TraceEvent{EventType: "turn.failed", Status: "error", ErrorType: "tool_binding_failed", Payload: map[string]any{"error": err.Error()}})
		out <- Event{Type: EventError, Content: err.Error()}
		return
	}
	todos := cftools.NewTodoStore()
	messages := e.buildMessages(ctx, req, todos)
	trace.noteCompression(ctx, len(messages), e.cfg.Runtime.MaxContextTurns)
	maxIterations := req.MaxIterations
	if maxIterations <= 0 {
		maxIterations = e.cfg.Runtime.MaxActions
	}
	if maxIterations <= 0 {
		maxIterations = 20
	}
	duplicates := map[string]int{}
	toolCalls, toolFailures, duplicateCount := 0, 0, 0
	for i := 0; i < maxIterations; i++ {
		iterationStart := time.Now()
		trace.record(ctx, storage.TraceEvent{EventType: "llm.iteration.started", Status: "ok", Iteration: i + 1, Payload: map[string]any{"message_count": len(messages)}})
		out <- Event{Type: EventIteration, Content: fmt.Sprintf("正在进行第 %d 轮模型调用", i+1), Iteration: i + 1}
		resp, err := e.adapter.Generate(ctx, modelWithTools, messages)
		if err != nil {
			trace.record(ctx, storage.TraceEvent{EventType: "llm.iteration.failed", Status: "error", Iteration: i + 1, ErrorType: "model_error", DurationMS: time.Since(iterationStart).Milliseconds(), Payload: map[string]any{"error": err.Error()}})
			trace.record(ctx, storage.TraceEvent{EventType: "turn.failed", Status: "error", ErrorType: "model_error", DurationMS: time.Since(start).Milliseconds()})
			e.logger.ErrorContext(ctx, "model request failed",
				slog.String("component", "engine"),
				slog.String("event", "model.request.failed"),
				slog.String("request_id", observability.RequestIDFromContext(ctx)),
				slog.String("session_id", req.SessionID),
				slog.String("error", err.Error()),
			)
			out <- Event{Type: EventError, Content: err.Error()}
			if e.runs != nil && req.RunID != "" {
				_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventError, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"error": err.Error()}})
			}
			return
		}
		trace.record(ctx, storage.TraceEvent{EventType: "llm.iteration.completed", Status: "ok", Iteration: i + 1, DurationMS: time.Since(iterationStart).Milliseconds(), Payload: map[string]any{"output_chars": len(resp.Content), "tool_calls": len(resp.ToolCalls)}})
		messages = append(messages, schema.AssistantMessage(resp.Content, resp.ToolCalls))
		if len(resp.ToolCalls) == 0 {
			content := strings.TrimSpace(resp.Content)
			e.recordMessage(ctx, storage.MessageRecord{SessionID: req.SessionID, RequestID: req.RequestID, Role: string(roleAssistant), Content: content})
			out <- Event{Type: EventOutput, Content: content}
			if e.runs != nil && req.RunID != "" {
				_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventModelEnd, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"output_chars": len(content), "iterations": i + 1}})
				_, _ = e.runs.Finish(ctx, run.AgentRun{ID: req.RunID, SessionID: req.SessionID, WorkspaceID: req.WorkspaceID, PlanID: req.PlanID, ModelProvider: e.cfg.Provider, ModelName: e.cfg.Model}, run.StatusCompleted, "")
			}
			trace.record(ctx, storage.TraceEvent{EventType: "turn.completed", Status: "ok", DurationMS: time.Since(start).Milliseconds(), Payload: map[string]any{"iterations": i + 1, "tool_calls": toolCalls, "tool_failures": toolFailures, "duplicates": duplicateCount, "output_chars": len(content)}})
			out <- Event{Type: EventStats, Content: fmt.Sprintf("duration=%s iterations=%d tools=%d duplicates=%d failures=%d", time.Since(start).Round(time.Millisecond), i+1, toolCalls, duplicateCount, toolFailures)}
			return
		}
		e.recordMessage(ctx, storage.MessageRecord{SessionID: req.SessionID, RequestID: req.RequestID, Role: string(roleAssistant), Content: resp.Content})
		for _, call := range resp.ToolCalls {
			toolCalls++
			toolStart := time.Now()
			callKey := duplicateKey(call.Function.Name, call.Function.Arguments)
			duplicates[callKey]++
			callCount := duplicates[callKey]
			if callCount >= 2 {
				duplicateCount++
				trace.record(ctx, storage.TraceEvent{EventType: "tool.call.duplicate_detected", Status: "warning", Iteration: i + 1, ToolName: call.Function.Name, ToolCallID: call.ID, Payload: map[string]any{"args_hash": argsHash(call.Function.Arguments), "count": callCount}})
			}
			trace.record(ctx, storage.TraceEvent{EventType: "tool.call.started", Status: "ok", Iteration: i + 1, ToolName: call.Function.Name, ToolCallID: call.ID, Payload: map[string]any{"args_hash": argsHash(call.Function.Arguments), "args_summary": argsSummary(call.Function.Arguments)}})
			out <- Event{Type: EventToolStarted, Content: fmt.Sprintf("正在调用 %s", call.Function.Name), ToolName: call.Function.Name, ToolCallID: call.ID, Iteration: i + 1}
			if e.runs != nil && req.RunID != "" {
				_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventToolStart, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"tool": call.Function.Name, "tool_call_id": call.ID, "iteration": i + 1}})
			}
			var result cftools.ToolResult
			if callCount >= 3 {
				result = cftools.WarningToolResult(fmt.Sprintf(`{"warning":"duplicate tool call suppressed","tool":%q,"count":%d}`, call.Function.Name, callCount), "duplicate_tool_call")
				result.DurationMS = time.Since(toolStart).Milliseconds()
				trace.record(ctx, storage.TraceEvent{EventType: "tool.call.warning", Status: "warning", Iteration: i + 1, ToolName: call.Function.Name, ToolCallID: call.ID, ErrorType: result.ErrorType, DurationMS: result.DurationMS, Payload: map[string]any{"count": callCount}})
			} else {
				result, _ = e.registry.Dispatch(ctx, call, cftools.ToolRuntime{
					WorkspaceID: req.WorkspaceID,
					ProjectRoot: req.ProjectRoot,
					SessionID:   req.SessionID,
					RequestID:   req.RequestID,
					PlanStepID:  "",
					Executor:    e.executor,
					Todos:       todos,
				})
				status := "ok"
				eventType := "tool.call.completed"
				if !result.OK {
					status = "error"
					eventType = "tool.call.failed"
					toolFailures++
				}
				trace.record(ctx, storage.TraceEvent{EventType: eventType, Status: status, Iteration: i + 1, ToolName: call.Function.Name, ToolCallID: call.ID, ErrorType: result.ErrorType, DurationMS: result.DurationMS, Payload: map[string]any{"result_chars": len(result.Content), "retryable": result.Retryable}})
				progressType := EventToolCompleted
				if !result.OK {
					progressType = EventToolFailed
				}
				if e.runs != nil && req.RunID != "" {
					eventType := run.EventToolEnd
					if !result.OK {
						eventType = run.EventToolError
					}
					_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: eventType, Timestamp: time.Now().UTC(), RequestID: req.RequestID, LatencyMS: result.DurationMS, Payload: map[string]any{"tool": call.Function.Name, "tool_call_id": call.ID, "ok": result.OK, "error_type": result.ErrorType}})
				}
				out <- Event{Type: progressType, Content: fmt.Sprintf("%s 完成，用时 %dms", call.Function.Name, result.DurationMS), ToolName: call.Function.Name, ToolCallID: call.ID, Iteration: i + 1, DurationMillis: result.DurationMS}
			}
			toolContent := strings.TrimSpace(result.Content)
			if toolContent == "" {
				toolContent = "{}"
			}
			messages = append(messages, schema.ToolMessage(toolContent, call.ID, schema.WithToolName(call.Function.Name)))
			e.recordMessage(ctx, storage.MessageRecord{
				SessionID:  req.SessionID,
				RequestID:  req.RequestID,
				Role:       "tool",
				Content:    toolContent,
				ToolCallID: call.ID,
				ToolName:   call.Function.Name,
			})
		}
		before := len(messages)
		messages = e.compressMessagesIfNeeded(messages, req, todos)
		if len(messages) < before {
			trace.record(ctx, storage.TraceEvent{EventType: "context.compressed", Status: "ok", Iteration: i + 1, Payload: map[string]any{"before_messages": before, "after_messages": len(messages)}})
		}
	}
	trace.record(ctx, storage.TraceEvent{EventType: "turn.failed", Status: "error", ErrorType: "budget_exhausted", DurationMS: time.Since(start).Milliseconds(), Payload: map[string]any{"max_iterations": maxIterations, "tool_calls": toolCalls, "tool_failures": toolFailures, "duplicates": duplicateCount}})
	out <- Event{Type: EventError, Content: fmt.Sprintf("agent iteration budget exhausted after %d iterations", maxIterations)}
	if e.runs != nil && req.RunID != "" {
		_ = e.runs.Event(ctx, run.RunEvent{RunID: req.RunID, Type: run.EventError, Timestamp: time.Now().UTC(), RequestID: req.RequestID, Payload: map[string]any{"error": "budget_exhausted"}})
		_, _ = e.runs.Finish(ctx, run.AgentRun{ID: req.RunID, SessionID: req.SessionID, WorkspaceID: req.WorkspaceID, PlanID: req.PlanID, ModelProvider: e.cfg.Provider, ModelName: e.cfg.Model}, run.StatusFailed, "budget exhausted")
	}
}

func (e *LLMEngine) consumeStream(ctx context.Context, stream *schema.StreamReader[*schema.Message], out chan<- Event, req Request, start time.Time) {
	defer stream.Close()
	var b strings.Builder
	chunks := 0
	for {
		msg, err := stream.Recv()
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "eof") {
				e.logger.ErrorContext(ctx, "model stream failed",
					slog.String("component", "engine"),
					slog.String("event", "model.stream.failed"),
					slog.String("request_id", observability.RequestIDFromContext(ctx)),
					slog.String("session_id", req.SessionID),
					slog.String("error", err.Error()),
				)
				out <- Event{Type: EventError, Content: err.Error()}
			}
			break
		}
		if msg == nil || msg.Content == "" {
			continue
		}
		chunks++
		b.WriteString(msg.Content)
		out <- Event{Type: EventToken, Content: msg.Content}
	}
	content := strings.TrimSpace(b.String())
	e.recordMessage(ctx, storage.MessageRecord{SessionID: req.SessionID, RequestID: req.RequestID, Role: string(roleAssistant), Content: content})
	duration := time.Since(start).Round(time.Millisecond)
	out <- Event{Type: EventStats, Content: fmt.Sprintf("duration=%s chunks=%d chars=%d", duration, chunks, len(content))}
	e.logger.InfoContext(ctx, "model stream completed",
		slog.String("component", "engine"),
		slog.String("event", "model.stream.completed"),
		slog.String("request_id", observability.RequestIDFromContext(ctx)),
		slog.String("session_id", req.SessionID),
		slog.Int("chunks", chunks),
		slog.Int("output_chars", len(content)),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)
}

func (e *LLMEngine) recordMessage(ctx context.Context, record storage.MessageRecord) {
	record.Role = strings.TrimSpace(record.Role)
	record.Content = strings.TrimSpace(record.Content)
	if record.Role == "" || (record.Content == "" && record.ToolName == "") {
		return
	}
	if e.messages != nil && record.SessionID != "" {
		_ = e.messages.AppendMessage(ctx, record)
	}
	if e.memory == nil || record.SessionID == "" {
		return
	}
	switch record.Role {
	case string(roleUser), string(roleAssistant), string(roleSystem):
		_ = e.memory.Append(ctx, record.SessionID, cfmemory.Turn{Role: record.Role, Content: record.Content})
	}
}

func (e *LLMEngine) toSchemaMessages(records []storage.MessageRecord) []*schema.Message {
	out := make([]*schema.Message, 0, len(records))
	for _, record := range records {
		switch strings.ToLower(strings.TrimSpace(record.Role)) {
		case string(roleAssistant):
			out = append(out, schema.AssistantMessage(record.Content, nil))
		case string(roleSystem):
			out = append(out, schema.SystemMessage(record.Content))
		case "tool":
			out = append(out, schema.AssistantMessage(fmt.Sprintf("[Tool result: %s]\n%s", record.ToolName, record.Content), nil))
		default:
			out = append(out, schema.UserMessage(record.Content))
		}
	}
	return out
}

func (e *LLMEngine) compressMessagesIfNeeded(messages []*schema.Message, req Request, todos *cftools.TodoStore) []*schema.Message {
	limit := e.cfg.Runtime.MaxContextTurns
	if limit <= 0 {
		limit = 40
	}
	if len(messages) <= limit+1 {
		return messages
	}
	keep := limit
	if keep < 8 {
		keep = 8
	}
	start := len(messages) - keep
	summary := "[Compression snapshot]\nOlder context was trimmed to keep the agent loop within budget."
	if todos != nil {
		if snapshot := todos.Snapshot(); snapshot != "" {
			summary += "\n" + snapshot
		}
	}
	if strings.TrimSpace(req.Input) != "" {
		summary += "\nLast user goal: " + strings.TrimSpace(req.Input)
	}
	if e.compressor != nil && e.messages != nil && req.SessionID != "" {
		if records, err := e.messages.ListMessages(context.Background(), req.SessionID, keep); err == nil {
			_, _ = e.compressor.Compress(context.Background(), req.SessionID, req.WorkspaceID, records)
		}
	}
	out := []*schema.Message{messages[0], schema.SystemMessage(summary)}
	out = append(out, messages[start:]...)
	return out
}

func (e *LLMEngine) persistPlan(req Request, content string) (*plan.Plan, error) {
	if e.plans == nil || strings.TrimSpace(req.SessionID) == "" {
		return nil, nil
	}
	type planPayload struct {
		Goal       string              `json:"goal"`
		Steps      []plan.PlanStep     `json:"steps"`
		Preference plan.PlanPreference `json:"preference"`
	}
	payload := planPayload{}
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") {
		_ = json.Unmarshal([]byte(trimmed), &payload)
	}
	if strings.TrimSpace(payload.Goal) == "" {
		payload.Goal = req.Input
	}
	if len(payload.Steps) == 0 {
		payload.Steps = []plan.PlanStep{{
			Title:       "Generated plan",
			Description: trimmed,
			Type:        plan.StepSummary,
			Status:      plan.StepPending,
		}}
	}
	status := plan.StatusPlanning
	for _, step := range payload.Steps {
		if step.RequiresApproval {
			status = plan.StatusWaitingApproval
			break
		}
	}
	return e.plans.Create(plan.Plan{
		ID:          req.PlanID,
		SessionID:   req.SessionID,
		WorkspaceID: req.WorkspaceID,
		Goal:        payload.Goal,
		Status:      status,
		Steps:       payload.Steps,
		Preference:  payload.Preference,
	})
}

type turnTrace struct {
	store     storage.TraceStore
	sessionID string
	requestID string
	rootSpan  string
}

func newTurnTrace(store storage.TraceStore, req Request) *turnTrace {
	return &turnTrace{
		store:     store,
		sessionID: req.SessionID,
		requestID: req.RequestID,
		rootSpan:  "turn_" + shortHash(req.RequestID+req.SessionID),
	}
}

func (t *turnTrace) record(ctx context.Context, event storage.TraceEvent) {
	if t == nil || t.store == nil {
		return
	}
	event.SessionID = t.sessionID
	event.RequestID = t.requestID
	if event.SpanID == "" {
		event.SpanID = t.rootSpan + "_" + shortHash(event.EventType+event.ToolCallID+fmt.Sprint(event.Iteration)+time.Now().String())
	}
	if event.ParentSpanID == "" && event.EventType != "turn.started" {
		event.ParentSpanID = t.rootSpan
	}
	_ = t.store.RecordTrace(ctx, event)
}

func (t *turnTrace) noteCompression(ctx context.Context, messageCount, limit int) {
	if limit <= 0 {
		limit = 40
	}
	if messageCount > limit+1 {
		t.record(ctx, storage.TraceEvent{EventType: "context.compressed", Status: "ok", Payload: map[string]any{"message_count": messageCount, "limit": limit}})
	}
}

func duplicateKey(name, args string) string {
	return strings.TrimSpace(name) + ":" + normalizedArgs(args)
}

func normalizedArgs(args string) string {
	var value any
	if err := json.Unmarshal([]byte(args), &value); err != nil {
		return strings.TrimSpace(args)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(args)
	}
	return string(data)
}

func argsHash(args string) string {
	return shortHash(normalizedArgs(args))
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func argsSummary(args string) string {
	normalized := normalizedArgs(args)
	if len(normalized) <= 180 {
		return normalized
	}
	return normalized[:180] + "..."
}

func (e *LLMEngine) prepareContext(ctx context.Context, req Request) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = observability.RequestIDFromContext(ctx)
	}
	if requestID != "" {
		ctx = observability.WithRequestID(ctx, requestID)
	}
	if strings.TrimSpace(req.SessionID) != "" {
		ctx = observability.WithSessionID(ctx, req.SessionID)
	}
	runType := "chat-model"
	if typ, ok := components.GetType(e.model); ok && strings.TrimSpace(typ) != "" {
		runType = typ
	}
	handler := callbacks.NewHandlerBuilder().
		OnStartFn(func(cbCtx context.Context, _ *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			modelInput := einomodel.ConvCallbackInput(input)
			messageCount := 0
			if modelInput != nil {
				messageCount = len(modelInput.Messages)
			}
			e.logger.InfoContext(cbCtx, "eino callback start",
				slog.String("component", "eino"),
				slog.String("event", "model.callback.start"),
				slog.String("request_id", observability.RequestIDFromContext(cbCtx)),
				slog.String("session_id", observability.SessionIDFromContext(cbCtx)),
				slog.String("run_type", runType),
				slog.Int("message_count", messageCount),
			)
			return cbCtx
		}).
		OnEndFn(func(cbCtx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			modelOutput := einomodel.ConvCallbackOutput(output)
			outputChars := 0
			if modelOutput != nil && modelOutput.Message != nil {
				outputChars = len(strings.TrimSpace(modelOutput.Message.Content))
			}
			e.logger.InfoContext(cbCtx, "eino callback end",
				slog.String("component", "eino"),
				slog.String("event", "model.callback.end"),
				slog.String("request_id", observability.RequestIDFromContext(cbCtx)),
				slog.String("session_id", observability.SessionIDFromContext(cbCtx)),
				slog.String("run_type", runType),
				slog.Int("output_chars", outputChars),
			)
			return cbCtx
		}).
		OnErrorFn(func(cbCtx context.Context, _ *callbacks.RunInfo, err error) context.Context {
			e.logger.ErrorContext(cbCtx, "eino callback error",
				slog.String("component", "eino"),
				slog.String("event", "model.callback.error"),
				slog.String("request_id", observability.RequestIDFromContext(cbCtx)),
				slog.String("session_id", observability.SessionIDFromContext(cbCtx)),
				slog.String("run_type", runType),
				slog.String("error", err.Error()),
			)
			return cbCtx
		}).
		Build()
	ctx = callbacks.InitCallbacks(ctx, &callbacks.RunInfo{
		Name:      "codeflow-engine",
		Type:      runType,
		Component: components.ComponentOfChatModel,
	}, handler)
	return callbacks.EnsureRunInfo(ctx, runType, components.ComponentOfChatModel)
}
