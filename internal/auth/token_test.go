package auth

import (
	"testing"
	"time"
)

func TestIssueMFA_CarriesPurpose(t *testing.T) {
	iss := NewTokenIssuer("secret", time.Hour)

	mfa, err := iss.IssueMFA("11111111-1111-1111-1111-111111111111", "t1")
	if err != nil {
		t.Fatalf("IssueMFA: %v", err)
	}
	claims, err := iss.Parse(mfa)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Pur != "mfa" {
		t.Fatalf("Pur=%q, want \"mfa\"", claims.Pur)
	}
	if claims.Subject != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("Subject=%q", claims.Subject)
	}
}

func TestIssue_FullSessionHasNoPurpose(t *testing.T) {
	iss := NewTokenIssuer("secret", time.Hour)
	tok, err := iss.Issue("11111111-1111-1111-1111-111111111111", "t1", "user", 0, "")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := iss.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Pur != "" {
		t.Fatalf("full session Pur=%q, want empty", claims.Pur)
	}
}
