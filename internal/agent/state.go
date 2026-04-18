package agent

import (
"github.com/cloudwego/eino/schema"
)

type AgentState struct {
Messages []*schema.Message `json:"messages"`
Summary  string           `json:"summary"`
ThreadID string           `json:"thread_id"`
}

type Turn struct {
UserMessage    string            `json:"user_message"`
AssistantReply string           `json:"assistant_reply"`
ToolCalls     []schema.ToolCall `json:"tool_calls,omitempty"`
ToolResults   []string          `json:"tool_results,omitempty"`
}

func NewAgentState(threadID string) *AgentState {
return &AgentState{
Messages: make([]*schema.Message, 0),
ThreadID: threadID,
}
}

func (s *AgentState) AddUserMessage(content string) {
s.Messages = append(s.Messages, schema.UserMessage(content))
}

func (s *AgentState) AddAssistantMessage(content string, toolCalls []schema.ToolCall) {
msg := schema.AssistantMessage(content, toolCalls)
s.Messages = append(s.Messages, msg)
}

func (s *AgentState) AddToolMessage(toolCallID, content string) {
s.Messages = append(s.Messages, schema.ToolMessage(content, toolCallID))
}

func (s *AgentState) TrimMessages(maxMessages int) ([]*schema.Message, []*schema.Message) {
if len(s.Messages) <= maxMessages {
return s.Messages, nil
}
keep := s.Messages[len(s.Messages)-maxMessages:]
discard := s.Messages[:len(s.Messages)-maxMessages]
return keep, discard
}

func (s *AgentState) GetSystemPrompt(extraInstructions string) string {
return extraInstructions
}
