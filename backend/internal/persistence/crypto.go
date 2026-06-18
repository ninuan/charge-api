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

type cookieCipher struct {
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

func newCookieCipher(key []byte) (*cookieCipher, error) {
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
	return &cookieCipher{aead: aead}, nil
}

func (c *cookieCipher) encrypt(userID string, plaintext string) ([]byte, []byte, error) {
	if plaintext == "" {
		return nil, nil, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate cookie nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, []byte(plaintext), []byte(userID))
	return nonce, ciphertext, nil
}

func (c *cookieCipher) decrypt(userID string, nonce []byte, ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(userID))
	if err != nil {
		return "", fmt.Errorf("decrypt cookie for user %s: %w", userID, err)
	}
	return string(plaintext), nil
}
