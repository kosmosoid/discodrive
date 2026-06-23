package subsonic

import (
	"testing"
)

// getUser must return a user object with role flags; clients like Feishin read
// adminRole on login and crash if the user object is missing.
func TestGetUser(t *testing.T) {
	h, _ := setupSubsonic(t)

	_, m := subsonicGet(h, "/rest/getUser?apiKey="+testAPIKey+"&c=test&v=1.16.1&f=json")
	resp := subsonicResponse(m)
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("expected ok, got: %v", resp)
	}
	u, ok := resp["user"].(map[string]any)
	if !ok || u == nil {
		t.Fatalf("missing user object: %v", resp)
	}
	if u["username"] != testEmail {
		t.Fatalf("username = %v, want %s", u["username"], testEmail)
	}
	if u["adminRole"] != false {
		t.Fatalf("adminRole = %v, want false (role=user)", u["adminRole"])
	}
	if u["streamRole"] != true {
		t.Fatalf("streamRole = %v, want true", u["streamRole"])
	}
}
