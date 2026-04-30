package observability

import "context"

type key string

const (
	requestIDKey key = "request_id"
	sessionIDKey key = "session_id"
	runIDKey     key = "run_id"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

func SessionIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(sessionIDKey).(string)
	return value
}

func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey, runID)
}

func RunIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(runIDKey).(string)
	return value
}
