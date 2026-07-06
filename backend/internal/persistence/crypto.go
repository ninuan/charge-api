package persistence

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const CookieKeySize = 32

type secretCipher struct {
	aead cipher.AEAD
}

func DecodeCookieKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("CHARGE_COOKIE_KEY is required")
	}

	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		key, err := encoding.DecodeString(encoded)
		if err == nil && len(key) == CookieKeySize {
			return key, nil
		}
	}
	return nil, fmt.Errorf("CHARGE_COOKIE_KEY must be a base64-encoded %d-byte key", CookieKeySize)
}

func newCookieCipher(key []byte) (*secretCipher, error) {
	if len(key) != CookieKeySize {
		return nil, fmt.Errorf("cookie encryption key must be %d bytes", CookieKeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cookie cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create cookie GCM: %w", err)
	}
	return &secretCipher{aead: aead}, nil
}

func (c *secretCipher) encryptWithAAD(aad string, plaintext []byte) ([]byte, []byte, error) {
	if len(plaintext) == 0 {
		return nil, nil, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate secret nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, plaintext, []byte(aad))
	return nonce, ciphertext, nil
}

func (c *secretCipher) decryptWithAAD(aad string, nonce []byte, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return nil, fmt.Errorf("decrypt secret for %s: %w", aad, err)
	}
	return plaintext, nil
}

func (c *secretCipher) encrypt(userID string, plaintext string) ([]byte, []byte, error) {
	nonce, ciphertext, err := c.encryptWithAAD(userID, []byte(plaintext))
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt cookie for user %s: %w", userID, err)
	}
	return nonce, ciphertext, nil
}

func (c *secretCipher) decrypt(userID string, nonce []byte, ciphertext []byte) (string, error) {
	plaintext, err := c.decryptWithAAD(userID, nonce, ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt cookie for user %s: %w", userID, err)
	}
	return string(plaintext), nil
}
