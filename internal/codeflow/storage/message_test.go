package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteMessageStoreAppendListSearchAndDelete(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "codeflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.Create(t.TempDir(), "test", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.AppendMessage(ctx, MessageRecord{SessionID: session.ID, RequestID: "r1", Role: "user", Content: "hello searchable world"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessage(ctx, MessageRecord{SessionID: session.ID, RequestID: "r1", Role: "assistant", Content: "reply"}); err != nil {
		t.Fatal(err)
	}
	messages, err := store.ListMessages(ctx, session.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
	results, err := store.SearchMessages(ctx, session.ID, "searchable", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Snippet == "" {
		t.Fatalf("unexpected search results: %+v", results)
	}
	if err := store.Delete(session.ProjectRoot, session.ID); err != nil {
		t.Fatal(err)
	}
	messages, err = store.ListMessages(ctx, session.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected messages to be deleted with session, got %+v", messages)
	}
}
