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
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, encryptedAPIKeyPrefix))
	if err != nil {
		return "", err
	}
	var lastErr error
	for _, key := range modelConfigDecryptKeys(dataDir) {
		plain, err := openAPIKeyPayload(key, raw)
		if err == nil {
			return plain, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no api key encryption key available")
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
	path := userSecretKeyPath()
	if key, err := readHexKeyFile(path); err == nil {
		return key, nil
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

func modelConfigDecryptKeys(dataDir string) [][]byte {
	keys := make([][]byte, 0, 2)
	if key, err := modelConfigSecretKey(dataDir); err == nil {
		keys = append(keys, key)
	}
	if strings.TrimSpace(dataDir) != "" {
		if key, err := readHexKeyFile(filepath.Join(dataDir, "secret.key")); err == nil {
			if len(keys) == 0 || hex.EncodeToString(keys[0]) != hex.EncodeToString(key) {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

func readHexKeyFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("invalid secret key file %s", path)
	}
	return decoded, nil
}

func openAPIKeyPayload(key, raw []byte) (string, error) {
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

func userSecretKeyPath() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".codeflow", "secret.key")
	}
	return filepath.Join(".codeflow", "secret.key")
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
