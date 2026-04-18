package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type MemoryManager struct {
	memoryDir     string
	profileDir    string
	turnMemoryDir string
	mu            sync.RWMutex
}

type UserProfile struct {
	Preferences map[string]string
	Facts       []string
	LastUpdated string
}

func NewMemoryManager(memoryDir string) *MemoryManager {
	mm := &MemoryManager{
		memoryDir:     memoryDir,
		profileDir:    filepath.Join(memoryDir, "profiles"),
		turnMemoryDir: filepath.Join(memoryDir, "turns"),
	}
	os.MkdirAll(mm.profileDir, 0755)
	os.MkdirAll(mm.turnMemoryDir, 0755)
	return mm
}

func (m *MemoryManager) GetUserProfile(threadID string) (*UserProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profilePath := filepath.Join(m.profileDir, threadID+".md")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &UserProfile{
				Preferences: make(map[string]string),
				Facts:       make([]string, 0),
			}, nil
		}
		return nil, err
	}

	profile := &UserProfile{
		Preferences: make(map[string]string),
		Facts:       make([]string, 0),
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			profile.Preferences[key] = value
		} else {
			profile.Facts = append(profile.Facts, line)
		}
	}

	return profile, nil
}

func (m *MemoryManager) SaveUserProfile(threadID string, profile *UserProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profilePath := filepath.Join(m.profileDir, threadID+".md")

	var sb strings.Builder
	sb.WriteString("# User Profile\n\n")

	for k, v := range profile.Preferences {
		sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}

	if len(profile.Facts) > 0 {
		sb.WriteString("\n## Facts\n")
		for _, fact := range profile.Facts {
			sb.WriteString(fmt.Sprintf("- %s\n", fact))
		}
	}

	return os.WriteFile(profilePath, []byte(sb.String()), 0644)
}

func (m *MemoryManager) AppendTurnMemory(threadID string, turn *TurnMemory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	turnPath := filepath.Join(m.turnMemoryDir, threadID+".jsonl")

	data, err := json.Marshal(turn)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(turnPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}

func (m *MemoryManager) GetRecentTurns(ctx context.Context, threadID string, maxTurns int) ([]*TurnMemory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	turnPath := filepath.Join(m.turnMemoryDir, threadID+".jsonl")

	data, err := os.ReadFile(turnPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	rawLines := strings.Split(string(data), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > maxTurns {
		lines = lines[len(lines)-maxTurns:]
	}

	turns := make([]*TurnMemory, 0, len(lines))
	for _, line := range lines {
		var turn TurnMemory
		if err := json.Unmarshal([]byte(line), &turn); err != nil {
			continue
		}
		turns = append(turns, &turn)
	}

	return turns, nil
}

type TurnMemory struct {
	TurnID       int      `json:"turn_id"`
	UserMsg      string   `json:"user_message"`
	AssistantMsg string   `json:"assistant_message"`
	ToolCalls    []string `json:"tool_calls,omitempty"`
	Timestamp    string   `json:"timestamp"`
}

func TrimMessagesByTurns(ctx context.Context, messages []*schema.Message, triggerTurns, keepTurns int) ([]*schema.Message, []*schema.Message, string) {
	var systemMsg *schema.Message
	var turns [][]*schema.Message
	var currentTurn []*schema.Message

	for _, msg := range messages {
		if msg.Role == schema.System {
			systemMsg = msg
			continue
		}

		if msg.Role == schema.User {
			if len(currentTurn) > 0 {
				turns = append(turns, currentTurn)
			}
			currentTurn = []*schema.Message{msg}
		} else {
			if len(currentTurn) > 0 {
				currentTurn = append(currentTurn, msg)
			}
		}
	}

	if len(currentTurn) > 0 {
		turns = append(turns, currentTurn)
	}

	if len(turns) < triggerTurns {
		if systemMsg != nil {
			result := []*schema.Message{systemMsg}
			result = append(result, flattenTurns(turns)...)
			return result, nil, ""
		}
		return flattenTurns(turns), nil, ""
	}

	recentTurns := turns[len(turns)-keepTurns:]
	discardedTurns := turns[:len(turns)-keepTurns]

	var recent []*schema.Message
	var discarded []*schema.Message

	if systemMsg != nil {
		recent = []*schema.Message{systemMsg}
	}

	for _, turn := range recentTurns {
		recent = append(recent, turn...)
	}
	for _, turn := range discardedTurns {
		discarded = append(discarded, turn...)
	}

	return recent, discarded, summarizeDiscarded(discardedTurns)
}

func flattenTurns(turns [][]*schema.Message) []*schema.Message {
	var result []*schema.Message
	for _, turn := range turns {
		result = append(result, turn...)
	}
	return result
}

func summarizeDiscarded(discardedTurns [][]*schema.Message) string {
	if len(discardedTurns) == 0 {
		return ""
	}
	return fmt.Sprintf("[%d turns were trimmed due to context length limits]", len(discardedTurns))
}
