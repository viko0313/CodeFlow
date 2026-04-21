package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

const RequestIDHeader = "X-Request-ID"

func RequestContextMiddleware(logger *slog.Logger) app.HandlerFunc {
	if logger == nil {
		logger = NewLogger("codeflow-web")
	}
	return func(ctx context.Context, c *app.RequestContext) {
		requestID := strings.TrimSpace(string(c.Request.Header.Peek(RequestIDHeader)))
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set("request_id", requestID)
		c.Response.Header.Set(RequestIDHeader, requestID)
		ctx = WithRequestID(ctx, requestID)
		start := time.Now()
		method := string(c.Method())
		path := c.FullPath()
		if path == "" {
			path = string(c.Path())
		}
		c.Next(ctx)
		logger.InfoContext(ctx, "http request completed",
			slog.String("component", "http"),
			slog.String("event", "http.request.completed"),
			slog.String("request_id", requestID),
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status", c.Response.StatusCode()),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
	}
}

func RequestIDFromHertz(c *app.RequestContext) string {
	if c == nil {
		return ""
	}
	if value := strings.TrimSpace(c.GetString("request_id")); value != "" {
		return value
	}
	return strings.TrimSpace(string(c.Request.Header.Peek(RequestIDHeader)))
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "req_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buffer)
}
