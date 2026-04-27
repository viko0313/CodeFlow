package web

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const encryptedAPIKeyPrefix = "enc:v1:"

func encryptAPIKey(dataDir, plaintext string) (string, string, error) {
	key, err := modelConfigSecretKey(dataDir)
	if err != nil {
		return "", "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return encryptedAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(payload), apiKeyHint(plaintext), nil
}

func decryptAPIKey(dataDir, ciphertext string) (string, error) {
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, encryptedAPIKeyPrefix) {
		return "", fmt.Errorf("unsupported encrypted api key format")
	}
	key, err := modelConfigSecretKey(dataDir)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, encryptedAPIKeyPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted api key payload is too short")
	}
	nonce := raw[:gcm.NonceSize()]
	sealed := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func modelConfigSecretKey(dataDir string) ([]byte, error) {
	if configured := strings.TrimSpace(os.Getenv("CODEFLOW_SECRET_KEY")); configured != "" {
		if decoded, err := hex.DecodeString(configured); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		if decoded, err := base64.RawStdEncoding.DecodeString(configured); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		if decoded, err := base64.StdEncoding.DecodeString(configured); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		if len(configured) == 32 {
			return []byte(configured), nil
		}
		return nil, fmt.Errorf("CODEFLOW_SECRET_KEY must be 32 bytes, hex, or base64 encoded")
	}
	if strings.TrimSpace(dataDir) == "" {
		dataDir = ".codeflow"
	}
	path := filepath.Join(dataDir, "secret.key")
	if data, err := os.ReadFile(path); err == nil {
		decoded, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("invalid secret key file %s", path)
		}
		return decoded, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func apiKeyHint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:3] + "-..." + value[len(value)-4:]
}
