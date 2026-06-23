package auth

import (
	"strings"
	"testing"
)

func TestNewUserCodeFormat(t *testing.T) {
	c := newUserCode()
	if len(c) != 9 || c[4] != '-' { // expected format: ABCD-EFGH
		t.Fatalf("user_code format: %q", c)
	}
	for _, r := range strings.ReplaceAll(c, "-", "") {
		if !strings.ContainsRune(userCodeAlphabet, r) {
			t.Fatalf("character outside alphabet: %q in %q", r, c)
		}
	}
	for _, bad := range "01OIL" {
		if strings.ContainsRune(userCodeAlphabet, bad) {
			t.Fatalf("ambiguous character %q found in alphabet", bad)
		}
	}
}

func TestNewDeviceTokenPrefixAndHashStable(t *testing.T) {
	tok, err := newDeviceToken()
	if err != nil {
		t.Fatalf("newDeviceToken: %v", err)
	}
	if !strings.HasPrefix(tok, "kfd_") {
		t.Fatalf("expected kfd_ prefix, got %q", tok)
	}
	if h1, h2 := tokenHash(tok), tokenHash(tok); h1 != h2 {
		t.Fatalf("token hash is not stable")
	}
	if tokenHash(tok) == tokenHash(tok+"x") {
		t.Fatalf("token hash does not depend on token value")
	}
}

func TestNewDeviceCodeUnique(t *testing.T) {
	a, _ := newDeviceCode()
	b, _ := newDeviceCode()
	if a == b {
		t.Fatalf("device_code is not unique")
	}
}
