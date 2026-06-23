package subsonic

import "testing"

func TestTokenInfo(t *testing.T) {
	h, _ := setupSubsonic(t)

	resp := doGet(h, testAPIKey, "tokenInfo", "")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	ti, _ := resp["tokenInfo"].(map[string]any)
	if ti == nil {
		t.Fatalf("no tokenInfo object: %v", resp)
	}
	if ti["username"] != testEmail {
		t.Errorf("username=%v, want %v", ti["username"], testEmail)
	}
}
