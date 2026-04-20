package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
	cfmemory "github.com/cloudwego/codeflow/internal/codeflow/memory"
	"github.com/cloudwego/codeflow/internal/model"
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
	ProjectRoot string
	Input       string
	AgentMD     string
}

type Engine interface {
	Run(ctx context.Context, req Request) (<-chan Event, error)
}

type LLMEngine struct {
	cfg    *cfconfig.Config
	model  einomodel.ChatModel
	memory cfmemory.ShortTermMemory
}

func New(ctx context.Context, cfg *cfconfig.Config, memory cfmemory.ShortTermMemory) (*LLMEngine, error) {
	legacyCfg := cfg.ToLegacy()
	chatModel, err := model.NewProviderManager().CreateChatModel(ctx, legacyCfg)
	if err != nil {
		return nil, err
	}
	return &LLMEngine{cfg: cfg, model: chatModel, memory: memory}, nil
}

func (e *LLMEngine) Run(ctx context.Context, req Request) (<-chan Event, error) {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		start := time.Now()
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
			out <- Event{Type: EventError, Content: err.Error()}
			return
		}
		content := strings.TrimSpace(resp.Content)
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "user", Content: req.Input})
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "assistant", Content: content})
		out <- Event{Type: EventOutput, Content: content}
		out <- Event{Type: EventStats, Content: fmt.Sprintf("duration=%s", time.Since(start).Round(time.Millisecond))}
	}()
	return out, nil
}

func (e *LLMEngine) messages(ctx context.Context, req Request) []*schema.Message {
	system := "You are CodeFlow Agent, a terminal-native enterprise coding assistant. Prefer concise, auditable steps. Do not modify files or run commands directly; ask the host tool executor to handle privileged operations."
	if strings.TrimSpace(req.AgentMD) != "" {
		system += "\n\n[Project rules from AGENT.md]\n" + strings.TrimSpace(req.AgentMD)
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
	for {
		msg, err := stream.Recv()
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "eof") {
				out <- Event{Type: EventError, Content: err.Error()}
			}
			break
		}
		if msg == nil || msg.Content == "" {
			continue
		}
		b.WriteString(msg.Content)
		out <- Event{Type: EventToken, Content: msg.Content}
	}
	content := strings.TrimSpace(b.String())
	if content != "" {
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "user", Content: req.Input})
		_ = e.memory.Append(ctx, req.SessionID, cfmemory.Turn{Role: "assistant", Content: content})
	}
	out <- Event{Type: EventStats, Content: fmt.Sprintf("duration=%s", time.Since(start).Round(time.Millisecond))}
}
