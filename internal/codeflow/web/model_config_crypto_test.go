package web

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestModelConfigSecretKeyUsesUserConfigBeforeProjectDataDir(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("CODEFLOW_SECRET_KEY", "")

	key, err := modelConfigSecretKey(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "secret.key")); !os.IsNotExist(err) {
		t.Fatalf("project secret.key should not be created by default: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codeflow", "secret.key")); err != nil {
		t.Fatalf("expected user-level secret key: %v", err)
	}
}

func TestModelConfigSecretKeyFallsBackToLegacyProjectKey(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("CODEFLOW_SECRET_KEY", "")
	legacyKey := make([]byte, 32)
	for i := range legacyKey {
		legacyKey[i] = byte(i + 1)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "secret.key"), []byte(hex.EncodeToString(legacyKey)), 0600); err != nil {
		t.Fatal(err)
	}

	userKey := make([]byte, 32)
	for i := range userKey {
		userKey[i] = byte(255 - i)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codeflow"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codeflow", "secret.key"), []byte(hex.EncodeToString(userKey)), 0600); err != nil {
		t.Fatal(err)
	}

	ciphertext := sealTestAPIKey(t, legacyKey, "legacy-secret")
	plain, err := decryptAPIKey(dataDir, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "legacy-secret" {
		t.Fatalf("expected legacy project key to decrypt existing secret, got %q", plain)
	}
}

func sealTestAPIKey(t *testing.T, key []byte, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return encryptedAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(append(nonce, sealed...))
}
