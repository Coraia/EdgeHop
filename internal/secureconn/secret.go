package secureconn

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const secretSize = 32

// LoadOrCreateSecret reads a pairing secret or atomically creates one.
func LoadOrCreateSecret(path string) ([]byte, string, error) {
	secret, code, err := LoadSecret(path)
	if err == nil {
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, "", fmt.Errorf("secureconn: protect pairing key: %w", err)
		}
		return secret, code, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	secret, err = GenerateSecret()
	if err != nil {
		return nil, "", err
	}
	code = EncodeSecret(secret)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, "", fmt.Errorf("secureconn: create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return LoadSecret(path)
	}
	if err != nil {
		return nil, "", fmt.Errorf("secureconn: create pairing key: %w", err)
	}
	if _, err := f.WriteString(code + "\n"); err != nil {
		f.Close()
		os.Remove(path)
		return nil, "", fmt.Errorf("secureconn: write pairing key: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return nil, "", fmt.Errorf("secureconn: close pairing key: %w", err)
	}
	return secret, code, nil
}

// LoadSecret reads and validates a pairing code file.
func LoadSecret(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	code := strings.TrimSpace(string(data))
	secret, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil {
		return nil, "", fmt.Errorf("secureconn: invalid pairing code: %w", err)
	}
	if len(secret) != secretSize {
		return nil, "", fmt.Errorf("secureconn: pairing key is %d bytes, want %d", len(secret), secretSize)
	}
	return secret, code, nil
}

// EncodeSecret formats a secret as a shell-safe pairing code.
func EncodeSecret(secret []byte) string {
	return base64.RawURLEncoding.EncodeToString(secret)
}
