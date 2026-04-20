package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Turn struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type ShortTermMemory interface {
	Append(ctx context.Context, sessionID string, turn Turn) error
	GetRecent(ctx context.Context, sessionID string, limit int64) ([]Turn, error)
	Clear(ctx context.Context, sessionID string) error
	Close() error
}

type RedisShortTermMemory struct {
	client *redis.Client
}

func NewRedisShortTermMemory(ctx context.Context, addr, password string, db int) (*RedisShortTermMemory, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("Redis is required for CodeFlow short-term memory; set CODEFLOW_REDIS_ADDR")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	return &RedisShortTermMemory{client: client}, nil
}

func (m *RedisShortTermMemory) Append(ctx context.Context, sessionID string, turn Turn) error {
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	key := keyFor(sessionID)
	pipe := m.client.TxPipeline()
	pipe.RPush(ctx, key, data)
	pipe.LTrim(ctx, key, -20, -1)
	_, err = pipe.Exec(ctx)
	return err
}

func (m *RedisShortTermMemory) GetRecent(ctx context.Context, sessionID string, limit int64) ([]Turn, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	raw, err := m.client.LRange(ctx, keyFor(sessionID), -limit, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Turn, 0, len(raw))
	for _, item := range raw {
		var turn Turn
		if err := json.Unmarshal([]byte(item), &turn); err == nil {
			out = append(out, turn)
		}
	}
	return out, nil
}

func (m *RedisShortTermMemory) Clear(ctx context.Context, sessionID string) error {
	return m.client.Del(ctx, keyFor(sessionID)).Err()
}

func (m *RedisShortTermMemory) Close() error {
	return m.client.Close()
}

func keyFor(sessionID string) string {
	return "codeflow:short_memory:" + sessionID
}
