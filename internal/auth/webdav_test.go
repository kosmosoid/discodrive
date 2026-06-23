package auth

import "testing"

func TestWebdavPasswordRoundTrip(t *testing.T) {
	plain, hash, err := newWebdavSecret()
	if err != nil {
		t.Fatalf("newWebdavSecret: %v", err)
	}
	if len(plain) < 16 {
		t.Fatalf("plain password too short: %q", plain)
	}
	ok, err := VerifyPassword(plain, hash)
	if err != nil || !ok {
		t.Fatalf("correct password did not verify: ok=%v err=%v", ok, err)
	}
	ok, _ = VerifyPassword("wrong-"+plain, hash)
	if ok {
		t.Fatal("wrong password passed verification")
	}
}
