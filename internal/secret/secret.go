// Package secret holds everything that touches key material: sealing upstream
// tokens at rest, hashing bearer credentials, and generating random tokens.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Box encrypts small secrets with AES-256-GCM.
type Box struct{ aead cipher.AEAD }

// NewBox derives an encryption key from arbitrary master key material.
func NewBox(master []byte) (*Box, error) {
	if len(master) < 16 {
		return nil, errors.New("secret key must be at least 16 bytes")
	}
	key, err := hkdf.Key(sha256.New, master, nil, "melon/token-encryption/v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The context (e.g. the row id) is bound as
// additional data, so a ciphertext cannot be moved to another row.
func (b *Box) Seal(plaintext, context string) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), []byte(context))
}

// Open reverses Seal.
func (b *Box) Open(sealed []byte, context string) (string, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("ciphertext too short")
	}
	out, err := b.aead.Open(nil, sealed[:n], sealed[n:], []byte(context))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// LoadMaster returns the master key from the environment value, or creates
// and persists a random one in dataDir when none is configured.
func LoadMaster(env, dataDir string) (key []byte, generated bool, err error) {
	if env = strings.TrimSpace(env); env != "" {
		return []byte(env), false, nil
	}
	path := filepath.Join(dataDir, "secret.key")
	if raw, err := os.ReadFile(path); err == nil {
		return []byte(strings.TrimSpace(string(raw))), false, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, false, err
	}
	enc := base64.StdEncoding.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(enc+"\n"), 0o600); err != nil {
		return nil, false, fmt.Errorf("persist generated secret: %w", err)
	}
	return []byte(enc), true, nil
}

// Token returns a random URL-safe token with the given prefix.
func Token(prefix string) string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf)
}

// ID returns a short random identifier with a readable prefix.
func ID(prefix string) string {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return prefix + "_" + strings.ToLower(strings.NewReplacer("-", "a", "_", "b").Replace(base64.RawURLEncoding.EncodeToString(buf)))
}

// Hash returns the hex SHA-256 of a high-entropy credential. Plain SHA-256 is
// appropriate here because the inputs are random 256-bit tokens, not passwords.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
