package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "antigravity-cli"
	userName    = "master-key"
)

// GetKey retrieves the master encryption key from the system keyring.
// If it doesn't exist, it generates a new one and saves it.
func GetKey() ([]byte, error) {
	// Try to get the key from the keyring
	keyStr, err := keyring.Get(serviceName, userName)
	if err == nil {
		return base64.StdEncoding.DecodeString(keyStr)
	}

	// If not found, generate a new key
	if errors.Is(err, keyring.ErrNotFound) {
		newKey := make([]byte, 32) // AES-256
		if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
			return nil, fmt.Errorf("failed to generate random key: %w", err)
		}

		keyBase64 := base64.StdEncoding.EncodeToString(newKey)
		if err := keyring.Set(serviceName, userName, keyBase64); err != nil {
			return nil, fmt.Errorf("failed to save key to keyring: %w", err)
		}

		return newKey, nil
	}

	return nil, fmt.Errorf("failed to retrieve key from keyring: %w", err)
}

// Encrypt encrypts data using AES-GCM.
func Encrypt(data []byte) ([]byte, error) {
	key, err := GetKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-GCM.
func Decrypt(data []byte) ([]byte, error) {
	key, err := GetKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
