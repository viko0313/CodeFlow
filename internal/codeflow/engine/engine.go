package engine

import (
	"context"
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
	"github.com/viko0313/CodeFlow/internal/model"
)

type EventType string

const (
	EventStatus EventType = "status"
	EventToken  EventType = "token"
	EventOutput EventType = "output"
	EventError  EventType = "error"
	EventStats  EventType = "stats"
)

type Event struct {
	Type    EventType
	Content string
}

type Request struct {
	SessionID   string
	RequestID   string
	ProjectRoot string
	Input       string
	AgentMD     string
	Context     string
	PlanEnabled bool
}

type Engine interface {
	Run(ctx context.Context, req Request) (<-chan Event, error)
}

type LLMEngine struct {
	cfg    *cfconfig.Config
	model  einomodel.ChatModel
	memory cfmemory.ShortTermMemory
	logger *slog.Logger
}

func New(ctx context.Context, cfg *cfconfig.Config, memory cfmemory.ShortTermMemory) (*LLMEngine, error) {
	legacyCfg := cfg.ToLegacy()
	chatModel, err := model.NewProviderManager().CreateChatModel(ctx, legacyCfg)
	if err != nil {
		return nil, err
	}
	return &LLMEngine{
		cfg:    cfg,
		model:  chatModel,
		memory: memory,
		logger: observability.NewLogger("codeflow-engine"),
	}, nil
}

func (e *LLMEngine) Run(ctx context.Context, req Request) (<-chan Event, error) {
	out := make(chan Event, 16)
	ctx = e.prepareContext(ctx, req)
	go func() {
		defer close(out)
		start := time.Now()
		e.logger.InfoContext(ctx, "model request started",
			slog.String("component", "engine"),
			slog.String("event", "model.request.started"),
			slog.String("request_id", observability.RequestIDFromContext(ctx)),
			slog.String("session_id", req.SessionID),
		)
		out <- Event{Type: EventStatus, Content: "thinking"}
		messages := e.messages(ctx, req)
		if streamer, ok := e.model.(interface {
			Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error)
		}); ok {
			if stream, err := streamer.Stream(ctx, messages); err == nil && stream != nil {
				e.consumeStream(ctx, stream, out, req, start)
				return
			}
		}
		resp, err := e.model.Generate(ctx, messages)
		if err != nil {
			e.logger.ErrorContext(ctx, "model request failed",
				slog.String("component", "engine"),
				slog.String("event", "model.request.failed"),
				slog.String("request_id", observability.RequestIDFromContext(ctx)),
				slog.String("session_id", req.SessionID),
				slog.String("error", err.Error()),
			)
			out <- Event{Type: EventError, Content: err.Error()}
			return
		}
		content := strings.TrimSpace(resp.Content)
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "user", Content: req.Input})
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "assistant", Content: content})
		out <- Event{Type: EventOutput, Content: content}
		out <- Event{Type: EventStats, Content: fmt.Sprintf("duration=%s", time.Since(start).Round(time.Millisecond))}
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

func (e *LLMEngine) messages(ctx context.Context, req Request) []*schema.Message {
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
	if turns, err := e.memory.GetRecent(ctx, req.SessionID, 20); err == nil && len(turns) > 0 {
		var b strings.Builder
		b.WriteString("\n\n[Recent session memory]\n")
		for _, turn := range turns {
			b.WriteString(turn.Role)
			b.WriteString(": ")
			b.WriteString(turn.Content)
			b.WriteString("\n")
		}
		system += b.String()
	}
	return []*schema.Message{
		schema.SystemMessage(system),
		schema.UserMessage(req.Input),
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
	if content != "" {
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "user", Content: req.Input})
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "assistant", Content: content})
	}
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
