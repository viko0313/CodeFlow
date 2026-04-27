package storage

import "testing"

func TestSQLiteModelConfigUpsertPreservesStoredAPIKeyWhenEmpty(t *testing.T) {
	store, err := NewSQLiteSessionStore(t.TempDir() + "/codeflow.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cipher := "ciphertext-one"
	hint := "sk-...1234"
	first, err := store.UpsertModelConfig("root-a", UpsertModelConfigInput{
		Provider:         "openai",
		Model:            "gpt-4.1",
		BaseURL:          "https://api.openai.com/v1",
		APIKeyCiphertext: &cipher,
		APIKeyHint:       &hint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.APIKeyCiphertext != cipher || first.APIKeyHint != hint {
		t.Fatalf("unexpected first config: %+v", first)
	}

	updated, err := store.UpsertModelConfig("root-a", UpsertModelConfigInput{
		Provider: "qwen",
		Model:    "qwen-plus",
		BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.APIKeyCiphertext != cipher {
		t.Fatalf("expected ciphertext to be preserved, got %q", updated.APIKeyCiphertext)
	}
	if updated.APIKeyHint != hint {
		t.Fatalf("expected hint to be preserved, got %q", updated.APIKeyHint)
	}
	if updated.Provider != "qwen" || updated.Model != "qwen-plus" {
		t.Fatalf("model fields were not updated: %+v", updated)
	}
}
