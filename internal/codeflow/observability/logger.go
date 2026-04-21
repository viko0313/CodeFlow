package observability

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(service string) *slog.Logger {
	level := parseLevel(os.Getenv("CODEFLOW_LOG_LEVEL"))
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	})
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = "local"
	}
	service = strings.TrimSpace(service)
	if service == "" {
		service = "codeflow"
	}
	return slog.New(handler).With(
		slog.String("service", service),
		slog.String("env", env),
	)
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
