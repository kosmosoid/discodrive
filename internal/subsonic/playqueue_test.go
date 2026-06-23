package subsonic

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

func TestSaveAndGetPlayQueue(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n1 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "s1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "s2.mp3", false)
	_, _, s1 := seed(t, ctx, h.q, userAID, n1, "Artist", "Album", "Song One")
	_, _, s2 := seed(t, ctx, h.q, userAID, n2, "Artist", "Album", "Song Two")

	id1 := encID("tr", db.UUIDString(s1.ID))
	id2 := encID("tr", db.UUIDString(s2.ID))

	save := doGet(h, testAPIKey, "savePlayQueue", "id="+id1+"&id="+id2+"&current="+id1+"&position=5000")
	if save["status"] != "ok" {
		t.Fatalf("savePlayQueue status=%v, want ok", save["status"])
	}

	get := doGet(h, testAPIKey, "getPlayQueue", "")
	if get["status"] != "ok" {
		t.Fatalf("getPlayQueue status=%v, want ok", get["status"])
	}
	pq, _ := get["playQueue"].(map[string]any)
	if pq == nil {
		t.Fatalf("no playQueue: %v", get)
	}
	if pq["current"] != id1 {
		t.Errorf("current=%v, want %v", pq["current"], id1)
	}
	entries, _ := pq["entry"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entry count=%d, want 2", len(entries))
	}
	// Verify position roundtrips correctly (JSON numbers decode as float64).
	if p, _ := pq["position"].(float64); int(p) != 5000 {
		t.Errorf("position=%v, want 5000", pq["position"])
	}
}

func TestGetPlayQueueEmpty(t *testing.T) {
	h, _ := setupSubsonic(t)
	get := doGet(h, testAPIKey, "getPlayQueue", "")
	if get["status"] != "ok" {
		t.Errorf("status=%v, want ok for empty queue", get["status"])
	}
}

// TestSavePlayQueueOverwrites verifies that a second save replaces the first.
func TestSavePlayQueueOverwrites(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n1 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "ow1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "ow2.mp3", false)
	_, _, s1 := seed(t, ctx, h.q, userAID, n1, "Artist", "Album", "Overwrite One")
	_, _, s2 := seed(t, ctx, h.q, userAID, n2, "Artist", "Album", "Overwrite Two")

	id1 := encID("tr", db.UUIDString(s1.ID))
	id2 := encID("tr", db.UUIDString(s2.ID))

	// Save a 2-track queue.
	if r := doGet(h, testAPIKey, "savePlayQueue", "id="+id1+"&id="+id2); r["status"] != "ok" {
		t.Fatalf("first save: status=%v", r["status"])
	}
	// Overwrite with a 1-track queue.
	if r := doGet(h, testAPIKey, "savePlayQueue", "id="+id1); r["status"] != "ok" {
		t.Fatalf("second save: status=%v", r["status"])
	}

	get := doGet(h, testAPIKey, "getPlayQueue", "")
	if get["status"] != "ok" {
		t.Fatalf("getPlayQueue status=%v", get["status"])
	}
	pq, _ := get["playQueue"].(map[string]any)
	entries, _ := pq["entry"].([]any)
	if len(entries) != 1 {
		t.Errorf("entry count=%d, want 1 (second save should replace first)", len(entries))
	}
}

// TestSavePlayQueueEmptyClears verifies that saving with no ids empties the queue.
func TestSavePlayQueueEmptyClears(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n1 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "cl1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "cl2.mp3", false)
	_, _, s1 := seed(t, ctx, h.q, userAID, n1, "Artist", "Album", "Clear One")
	_, _, s2 := seed(t, ctx, h.q, userAID, n2, "Artist", "Album", "Clear Two")

	id1 := encID("tr", db.UUIDString(s1.ID))
	id2 := encID("tr", db.UUIDString(s2.ID))

	// Save a 2-track queue.
	if r := doGet(h, testAPIKey, "savePlayQueue", "id="+id1+"&id="+id2); r["status"] != "ok" {
		t.Fatalf("initial save: status=%v", r["status"])
	}
	// Save with no ids — should clear.
	if r := doGet(h, testAPIKey, "savePlayQueue", ""); r["status"] != "ok" {
		t.Fatalf("empty save: status=%v", r["status"])
	}

	get := doGet(h, testAPIKey, "getPlayQueue", "")
	if get["status"] != "ok" {
		t.Fatalf("getPlayQueue status=%v", get["status"])
	}
	pq, _ := get["playQueue"].(map[string]any)
	entries, _ := pq["entry"].([]any)
	if len(entries) != 0 {
		t.Errorf("entry count=%d, want 0 (empty save should clear queue)", len(entries))
	}
}

// TestPlayQueueUserIsolation verifies that user A's queue is not visible to user B.
func TestPlayQueueUserIsolation(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	// User B's apiKey is "apikeyB" as set by setupTwo.
	const apikeyB = "apikeyB"

	n1 := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "iso1.mp3", false)
	_, _, s1 := seed(t, ctx, h.q, userAID, n1, "Artist", "Album", "Isolation Song")
	id1 := encID("tr", db.UUIDString(s1.ID))

	// Save user A's queue.
	if r := doGet(h, testAPIKey, "savePlayQueue", "id="+id1+"&position=1234"); r["status"] != "ok" {
		t.Fatalf("savePlayQueue(A): status=%v", r["status"])
	}

	// User B should see an empty queue (no entries).
	get := doGet(h, apikeyB, "getPlayQueue", "")
	if get["status"] != "ok" {
		t.Fatalf("getPlayQueue(B): status=%v", get["status"])
	}
	pq, _ := get["playQueue"].(map[string]any)
	entries, _ := pq["entry"].([]any)
	if len(entries) != 0 {
		t.Errorf("user B sees %d entries from user A's queue, want 0", len(entries))
	}
}
