package subsonic

import "testing"

func radioStations(resp map[string]any) []any {
	wrap, _ := resp["internetRadioStations"].(map[string]any)
	if wrap == nil {
		return nil
	}
	st, _ := wrap["internetRadioStation"].([]any)
	return st
}

func TestRadioCreateAndList(t *testing.T) {
	h, _ := setupSubsonic(t)
	create := doGet(h, testAPIKey, "createInternetRadioStation",
		"streamUrl=http://example.com/stream&name=My%20Station&homepageUrl=http://example.com")
	if create["status"] != "ok" {
		t.Fatalf("create status=%v, want ok", create["status"])
	}
	stations := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))
	if len(stations) != 1 {
		t.Fatalf("station count=%d, want 1", len(stations))
	}
	s, _ := stations[0].(map[string]any)
	if s["name"] != "My Station" {
		t.Errorf("name=%v", s["name"])
	}
	if s["streamUrl"] != "http://example.com/stream" {
		t.Errorf("streamUrl=%v", s["streamUrl"])
	}
	if s["homePageUrl"] != "http://example.com" {
		t.Errorf("homePageUrl=%v (capital P)", s["homePageUrl"])
	}
	if id, _ := s["id"].(string); len(id) < 3 || id[:3] != "ir-" {
		t.Errorf("id=%v, want ir-<uuid>", s["id"])
	}
}

func TestRadioCreateMissingParam(t *testing.T) {
	h, _ := setupSubsonic(t)
	if doGet(h, testAPIKey, "createInternetRadioStation", "name=NoStream")["status"] != "failed" {
		t.Errorf("want failed when streamUrl missing")
	}
	if doGet(h, testAPIKey, "createInternetRadioStation", "streamUrl=http://x")["status"] != "failed" {
		t.Errorf("want failed when name missing")
	}
}

func TestRadioUpdate(t *testing.T) {
	h, _ := setupSubsonic(t)
	doGet(h, testAPIKey, "createInternetRadioStation", "streamUrl=http://a&name=Old")
	id, _ := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))[0].(map[string]any)["id"].(string)
	if doGet(h, testAPIKey, "updateInternetRadioStation", "id="+id+"&streamUrl=http://b&name=New")["status"] != "ok" {
		t.Fatalf("update not ok")
	}
	s, _ := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))[0].(map[string]any)
	if s["name"] != "New" || s["streamUrl"] != "http://b" {
		t.Errorf("after update: %v", s)
	}
}

func TestRadioDelete(t *testing.T) {
	h, _ := setupSubsonic(t)
	doGet(h, testAPIKey, "createInternetRadioStation", "streamUrl=http://a&name=Gone")
	id, _ := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))[0].(map[string]any)["id"].(string)
	if doGet(h, testAPIKey, "deleteInternetRadioStation", "id="+id)["status"] != "ok" {
		t.Fatalf("delete not ok")
	}
	if n := len(radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))); n != 0 {
		t.Errorf("count=%d, want 0", n)
	}
}

// TestRadioListEmpty verifies that getInternetRadioStations returns ok with an empty list when no stations exist.
func TestRadioListEmpty(t *testing.T) {
	h, _ := setupSubsonic(t)
	resp := doGet(h, testAPIKey, "getInternetRadioStations", "")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	if n := len(radioStations(resp)); n != 0 {
		t.Errorf("station count=%d, want 0", n)
	}
}

// TestRadioUpdateNotFound verifies that updateInternetRadioStation fails for a non-existent or malformed id.
func TestRadioUpdateNotFound(t *testing.T) {
	h, _ := setupSubsonic(t)
	// Well-formed id format but station does not exist.
	resp := doGet(h, testAPIKey, "updateInternetRadioStation",
		"id=ir-00000000-0000-0000-0000-000000000000&streamUrl=http://x&name=X")
	if resp["status"] != "failed" {
		t.Errorf("non-existent station: status=%v, want failed", resp["status"])
	}
	// Malformed id (no kind prefix).
	resp2 := doGet(h, testAPIKey, "updateInternetRadioStation",
		"id=bogus&streamUrl=http://x&name=X")
	if resp2["status"] != "failed" {
		t.Errorf("malformed id: status=%v, want failed", resp2["status"])
	}
}

// TestRadioUpdateUserIsolation verifies that user B cannot update user A's stations.
func TestRadioUpdateUserIsolation(t *testing.T) {
	h, _, _, _ := setupTwo(t)
	// User B's apiKey is "apikeyB" as set by setupTwo.
	const apikeyB = "apikeyB"

	// User A creates a station.
	doGet(h, testAPIKey, "createInternetRadioStation", "streamUrl=http://original&name=Original")
	stations := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))
	if len(stations) != 1 {
		t.Fatalf("user A station count=%d, want 1", len(stations))
	}
	id, _ := stations[0].(map[string]any)["id"].(string)

	// User B attempts to update user A's station.
	if doGet(h, apikeyB, "updateInternetRadioStation", "id="+id+"&streamUrl=http://hijacked&name=Hijacked")["status"] != "failed" {
		t.Errorf("user B should not be able to update user A's station")
	}

	// User A's station should be unchanged.
	updated := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))
	if len(updated) != 1 {
		t.Fatalf("user A station count=%d after B's update attempt, want 1", len(updated))
	}
	s, _ := updated[0].(map[string]any)
	if s["name"] != "Original" || s["streamUrl"] != "http://original" {
		t.Errorf("user A station modified by user B: name=%v streamUrl=%v", s["name"], s["streamUrl"])
	}
}

// TestRadioUserIsolation verifies that user B cannot see or delete user A's stations.
func TestRadioUserIsolation(t *testing.T) {
	h, _, _, _ := setupTwo(t)
	// User B's apiKey is "apikeyB" as set by setupTwo.
	const apikeyB = "apikeyB"

	// User A creates a station.
	doGet(h, testAPIKey, "createInternetRadioStation", "streamUrl=http://a&name=AStation")
	stations := radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))
	if len(stations) != 1 {
		t.Fatalf("user A station count=%d, want 1", len(stations))
	}
	id, _ := stations[0].(map[string]any)["id"].(string)

	// User B should see no stations.
	if n := len(radioStations(doGet(h, apikeyB, "getInternetRadioStations", ""))); n != 0 {
		t.Errorf("user B sees %d stations from user A, want 0", n)
	}

	// User B cannot delete user A's station.
	if doGet(h, apikeyB, "deleteInternetRadioStation", "id="+id)["status"] != "failed" {
		t.Errorf("user B should not be able to delete user A's station")
	}

	// User A still has 1 station.
	if n := len(radioStations(doGet(h, testAPIKey, "getInternetRadioStations", ""))); n != 1 {
		t.Errorf("user A station count=%d after B's delete attempt, want 1", n)
	}
}
