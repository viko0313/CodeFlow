package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

type TokenUsage struct {
	Prompt     int  `json:"prompt"`
	Completion int  `json:"completion"`
	Total      int  `json:"total"`
	Known      bool `json:"known"`
}

type RuntimeStats struct {
	Provider       string        `json:"provider"`
	Model          string        `json:"model"`
	TokenUsage     TokenUsage    `json:"token_usage"`
	ModelDuration  time.Duration `json:"model_duration"`
	TotalDuration  time.Duration `json:"total_duration"`
	Flow           []string      `json:"flow"`
	Tools          []string      `json:"tools"`
	RecentTurns    int           `json:"recent_turns"`
	ProfileLoaded  bool          `json:"profile_loaded"`
	SandboxEnabled bool          `json:"sandbox_enabled"`
	Iterations     int           `json:"iterations"`
}

type RunResult struct {
	Response    string
	ToolResults []string
	Stats       RuntimeStats
}

func (u *TokenUsage) AddFromMessage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	u.Known = true
	u.Prompt += msg.ResponseMeta.Usage.PromptTokens
	u.Completion += msg.ResponseMeta.Usage.CompletionTokens
	u.Total += msg.ResponseMeta.Usage.TotalTokens
}

func (s RuntimeStats) TokenSummary() string {
	if !s.TokenUsage.Known {
		return "unknown"
	}
	return fmt.Sprintf("prompt=%d completion=%d total=%d turn=%d", s.TokenUsage.Prompt, s.TokenUsage.Completion, s.TokenUsage.Total, s.TokenUsage.Total)
}

func (s RuntimeStats) FlowSummary() string {
	if len(s.Flow) == 0 {
		return "Input -> Model -> Output"
	}
	return strings.Join(s.Flow, " -> ")
}

func (s RuntimeStats) ToolsSummary() string {
	if len(s.Tools) == 0 {
		return "none"
	}
	return strings.Join(s.Tools, ", ")
}
