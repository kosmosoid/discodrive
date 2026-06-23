package secret_test

import (
	"testing"

	"discodrive/internal/secret"
)

func TestEncryptRoundTrip(t *testing.T) {
	c, err := secret.New("0123456789abcdef0123456789abcdef") // 32 bytes
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	enc, err := c.Encrypt("hunter2")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "hunter2" {
		t.Fatal("ciphertext matched the plaintext")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "hunter2" {
		t.Fatalf("round-trip: got %q", dec)
	}
}

func TestEncryptNonceVaries(t *testing.T) {
	c, _ := secret.New("0123456789abcdef0123456789abcdef")
	a, _ := c.Encrypt("x")
	b, _ := c.Encrypt("x")
	if a == b {
		t.Fatal("two encryptions of the same text matched (nonce does not change)")
	}
}

func TestWrongKeyFails(t *testing.T) {
	c1, _ := secret.New("0123456789abcdef0123456789abcdef")
	enc, _ := c1.Encrypt("secret")
	c2, _ := secret.New("ffffffffffffffffffffffffffffffff")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("decryption with a different key must fail")
	}
}

func TestNoKeyForbidsSecrets(t *testing.T) {
	c, err := secret.New("")
	if err != nil {
		t.Fatalf("New(\"\") must not fail at startup: %v", err)
	}
	if _, err := c.Encrypt("x"); err != secret.ErrNoKey {
		t.Fatalf("Encrypt without a key: expected ErrNoKey, got %v", err)
	}
	if _, err := c.Decrypt("x"); err != secret.ErrNoKey {
		t.Fatalf("Decrypt without a key: expected ErrNoKey, got %v", err)
	}
}

func TestBadKeyLength(t *testing.T) {
	if _, err := secret.New("too-short"); err == nil {
		t.Fatal("a key of the wrong length must return an error")
	}
}
