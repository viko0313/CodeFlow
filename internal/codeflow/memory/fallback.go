package memory

import (
	"context"
	"strings"
)

const (
	BackendRedis  = "redis"
	BackendMemory = "memory"
)

func OpenShortTermMemoryWithFallback(ctx context.Context, addr, password string, db int) (ShortTermMemory, string, bool, error) {
	if strings.TrimSpace(addr) != "" {
		redisMemory, err := NewRedisShortTermMemory(ctx, addr, password, db)
		if err == nil {
			return redisMemory, BackendRedis, false, nil
		}
		return NewInMemoryShortTermMemory(), BackendMemory, true, nil
	}
	return NewInMemoryShortTermMemory(), BackendMemory, true, nil
}
