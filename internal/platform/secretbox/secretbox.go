// Package secretbox encrypts channel secrets at rest with AES-256-GCM.
//
// It is a platform seam rather than a helper inside the channel service because
// encryption has its own failure modes worth testing separately, and because a
// second consumer (an OAuth token in a later phase) is already foreseeable.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrNoKey means no encryption key is configured. Callers translate this into
// "channels with secrets are unavailable in this deployment" rather than failing
// the whole application: a box install owes no configuration for in-app alone.
var ErrNoKey = errors.New("secretbox: no encryption key configured")

// keyLen is 32 bytes — AES-256.
const keyLen = 32

type Box struct{ aead cipher.AEAD }

// New builds a Box from a base64-encoded 32-byte key, normally read from
// NOTIFICATIONS_SECRET_KEY.
func New(keyB64 string) (*Box, error) {
	if keyB64 == "" {
		return nil, ErrNoKey
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("secretbox: key is not valid base64: %w", err)
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", keyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The nonce is random per call and stored as the
// ciphertext's prefix, so two tenants holding the same secret produce different
// bytes and cannot be correlated by comparing rows.
func (b *Box) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secretbox: nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open decrypts. GCM authenticates, so a tampered or truncated value returns an
// error rather than plausible garbage.
func (b *Box) Open(ciphertext []byte) (string, error) {
	ns := b.aead.NonceSize()
	if len(ciphertext) < ns {
		return "", errors.New("secretbox: ciphertext shorter than nonce")
	}
	out, err := b.aead.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: %w", err)
	}
	return string(out), nil
}

// Hint renders the mask shown in the admin UI in place of a secret: the last four
// characters, prefixed by dots. A secret too short to mask is hidden entirely —
// showing "••ab" of a four-character token would give away most of it.
func Hint(plaintext string) string {
	const tail = 4
	if plaintext == "" {
		return ""
	}
	r := []rune(plaintext)
	if len(r) <= tail*2 {
		return "••••"
	}
	return "••••" + string(r[len(r)-tail:])
}
