package redis

import "github.com/viko0313/CodeFlow/internal/config"

func NewRedisClientFromConfig(_ *config.Config) (*SessionManager, error) {
	sm := &SessionManager{}
	_ = sm.Init(nil)
	return sm, nil
}

func CreateRedisClient(cfg *config.Config) (*SessionManager, error) {
	return NewRedisClientFromConfig(cfg)
}
