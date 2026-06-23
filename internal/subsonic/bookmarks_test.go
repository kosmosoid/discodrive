package subsonic

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// firstBookmark returns the bookmark list from a getBookmarks response.
func bookmarkList(t *testing.T, resp map[string]any) []any {
	t.Helper()
	wrapper, ok := resp["bookmarks"].(map[string]any)
	if !ok {
		t.Fatalf("no bookmarks wrapper in response: %v", resp)
	}
	list, _ := wrapper["bookmark"].([]any)
	return list
}

func TestBookmarkSongRoundTrip(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)

	nodeID := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "song.mp3", false)
	_, _, song := seed(t, ctx, h.q, userAID, nodeID, "Boards of Canada", "Geogaddi", "Music Is Math")
	trID := encID("tr", db.UUIDString(song.ID))

	resp := doGet(h, testAPIKey, "createBookmark", "id="+trID+"&position=5000&comment=hi")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("createBookmark failed: %v", resp)
	}

	resp = doGet(h, testAPIKey, "getBookmarks", "")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getBookmarks failed: %v", resp)
	}

	list := bookmarkList(t, resp)
	if len(list) != 1 {
		t.Fatalf("want 1 bookmark, got %d: %v", len(list), list)
	}

	bm, _ := list[0].(map[string]any)
	if bm == nil {
		t.Fatalf("bookmark is not an object: %v", list[0])
	}
	if pos, _ := bm["position"].(float64); pos != 5000 {
		t.Errorf("position = %v, want 5000", bm["position"])
	}
	if bm["comment"] != "hi" {
		t.Errorf("comment = %v, want hi", bm["comment"])
	}
	entry, _ := bm["entry"].(map[string]any)
	if entry == nil {
		t.Fatalf("no entry in bookmark: %v", bm)
	}
	if entry["id"] != trID {
		t.Errorf("entry.id = %v, want %s", entry["id"], trID)
	}
}

func TestBookmarkDelete(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)

	nodeID := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "song.mp3", false)
	_, _, song := seed(t, ctx, h.q, userAID, nodeID, "Aphex Twin", "SAW II", "Rhubarb")
	trID := encID("tr", db.UUIDString(song.ID))

	if resp := doGet(h, testAPIKey, "createBookmark", "id="+trID+"&position=1000"); resp["status"] != "ok" {
		t.Fatalf("createBookmark failed: %v", resp)
	}
	if resp := doGet(h, testAPIKey, "deleteBookmark", "id="+trID); resp["status"] != "ok" {
		t.Fatalf("deleteBookmark failed: %v", resp)
	}

	resp := doGet(h, testAPIKey, "getBookmarks", "")
	if resp["status"] != "ok" {
		t.Fatalf("getBookmarks failed: %v", resp)
	}
	if list := bookmarkList(t, resp); len(list) != 0 {
		t.Errorf("want 0 bookmarks after delete, got %d: %v", len(list), list)
	}
}

func TestBookmarkUserIsolation(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)

	nodeID := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "song.mp3", false)
	_, _, song := seed(t, ctx, h.q, userAID, nodeID, "Burial", "Untrue", "Archangel")
	trID := encID("tr", db.UUIDString(song.ID))

	if resp := doGet(h, testAPIKey, "createBookmark", "id="+trID+"&position=2000"); resp["status"] != "ok" {
		t.Fatalf("createBookmark(A) failed: %v", resp)
	}

	// User B (apikeyB, set by setupTwo) must not see A's bookmark.
	resp := doGet(h, "apikeyB", "getBookmarks", "")
	if resp["status"] != "ok" {
		t.Fatalf("getBookmarks(B) failed: %v", resp)
	}
	if list := bookmarkList(t, resp); len(list) != 0 {
		t.Errorf("user B should see 0 bookmarks, got %d: %v", len(list), list)
	}
}
