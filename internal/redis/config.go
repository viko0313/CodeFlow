package redis

import "github.com/cloudwego/codeflow/internal/config"

type Config struct {
	Host          string
	Port          int
	DB            int
	SessionPrefix string
}

var SessionConfig *Config

func InitSessionConfig(_ *config.Config) error {
	SessionConfig = &Config{}
	return nil
}

func GetSessionConfig() *Config {
	return SessionConfig
}
