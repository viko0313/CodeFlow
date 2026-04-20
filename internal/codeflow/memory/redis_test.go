package memory

import (
	"context"
	"os"
	"strconv"
	"testing"
)

func TestRedisMemoryRequiresAddress(t *testing.T) {
	if _, err := NewRedisShortTermMemory(context.Background(), "", "", 0); err == nil {
		t.Fatal("expected missing redis address error")
	}
}

func TestRedisShortTermMemoryIntegration(t *testing.T) {
	addr := os.Getenv("CODEFLOW_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set CODEFLOW_TEST_REDIS_ADDR to run integration test")
	}
	db := 0
	if raw := os.Getenv("CODEFLOW_TEST_REDIS_DB"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatal(err)
		}
		db = parsed
	}
	mem, err := NewRedisShortTermMemory(context.Background(), addr, os.Getenv("CODEFLOW_TEST_REDIS_PASSWORD"), db)
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	sessionID := "test-session"
	_ = mem.Clear(context.Background(), sessionID)
	if err := mem.Append(context.Background(), sessionID, Turn{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	turns, err := mem.GetRecent(context.Background(), sessionID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Content != "hello" {
		t.Fatalf("unexpected turns: %+v", turns)
	}
}
