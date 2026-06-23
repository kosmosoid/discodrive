// Package secret encrypts sensitive settings in the database using AES-256-GCM.
// The key is SETTINGS_ENCRYPTION_KEY (exactly 32 bytes). An empty key puts the Cipher
// into forbidden mode: all Encrypt/Decrypt calls return ErrNoKey (secrets must not
// be stored in plaintext).
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrNoKey is returned when trying to use secrets without a key configured.
var ErrNoKey = errors.New("secret encryption is not configured (SETTINGS_ENCRYPTION_KEY)")

// Cipher encrypts and decrypts strings. Created via New.
type Cipher struct {
	gcm cipher.AEAD // nil if no key is set
}

// New builds a Cipher. Empty key → a valid but "forbidden" Cipher (all ops return ErrNoKey).
// Non-empty key of the wrong length → error (fatal at startup).
func New(key string) (*Cipher, error) {
	if key == "" {
		return &Cipher{}, nil
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("SETTINGS_ENCRYPTION_KEY must be exactly 32 bytes, got %d", len(key))
	}
	if key == "change-me-to-a-32-byte-secret!!!" {
		return nil, errors.New("SETTINGS_ENCRYPTION_KEY is still the default placeholder — set a random 32-byte value")
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

// Enabled reports whether a key has been configured.
func (c *Cipher) Enabled() bool { return c.gcm != nil }

// Encrypt → base64(nonce || ciphertext).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if c.gcm == nil {
		return "", ErrNoKey
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt accepts base64(nonce || ciphertext).
func (c *Cipher) Decrypt(token string) (string, error) {
	if c.gcm == nil {
		return "", ErrNoKey
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", err
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext is too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
