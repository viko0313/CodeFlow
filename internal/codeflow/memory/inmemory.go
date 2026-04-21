package memory

import (
	"context"
	"sync"
	"time"
)

type InMemoryShortTermMemory struct {
	mu    sync.Mutex
	turns map[string][]Turn
}

func NewInMemoryShortTermMemory() *InMemoryShortTermMemory {
	return &InMemoryShortTermMemory{
		turns: map[string][]Turn{},
	}
}

func (m *InMemoryShortTermMemory) Append(_ context.Context, sessionID string, turn Turn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = time.Now().UTC()
	}
	items := append(m.turns[sessionID], turn)
	if len(items) > 20 {
		items = items[len(items)-20:]
	}
	m.turns[sessionID] = items
	return nil
}

func (m *InMemoryShortTermMemory) GetRecent(_ context.Context, sessionID string, limit int64) ([]Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	items := m.turns[sessionID]
	if len(items) == 0 {
		return []Turn{}, nil
	}
	start := len(items) - int(limit)
	if start < 0 {
		start = 0
	}
	out := make([]Turn, len(items[start:]))
	copy(out, items[start:])
	return out, nil
}

func (m *InMemoryShortTermMemory) Clear(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.turns, sessionID)
	return nil
}

func (m *InMemoryShortTermMemory) Close() error {
	return nil
}
