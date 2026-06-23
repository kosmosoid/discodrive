package subsonic

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// TestStarredAppearsInGetAlbum verifies that starring a song makes the `starred`
// field appear on that song object in a getAlbum response, and that unstarring removes it.
func TestStarredAppearsInGetAlbum(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	nodeID := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "star-song.mp3", false)
	_, album, song := seed(t, ctx, h.q, userUUID, nodeID, "StarArt", "StarAlb", "StarSong")
	songID := encID("tr", db.UUIDString(song.ID))
	albumID := encID("al", db.UUIDString(album.ID))

	// Star the song.
	resp := doGet(h, testAPIKey, "star", "id="+songID)
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("star failed: %v", resp)
	}

	// getAlbum should now include the starred song with a non-empty `starred` field.
	resp2 := doGet(h, testAPIKey, "getAlbum", "id="+albumID)
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("getAlbum failed: %v", resp2)
	}
	albumObj, _ := resp2["album"].(map[string]any)
	songs, _ := albumObj["song"].([]any)
	if len(songs) == 0 {
		t.Fatalf("no songs in album response")
	}
	songObj, _ := songs[0].(map[string]any)
	starredVal, hasStarred := songObj["starred"]
	if !hasStarred {
		t.Errorf("song object missing 'starred' field after starring: %v", songObj)
	}
	if s, _ := starredVal.(string); s == "" {
		t.Errorf("starred field is present but empty: %v", songObj)
	}

	// Unstar and verify the field disappears.
	resp3 := doGet(h, testAPIKey, "unstar", "id="+songID)
	if resp3 == nil || resp3["status"] != "ok" {
		t.Fatalf("unstar failed: %v", resp3)
	}

	resp4 := doGet(h, testAPIKey, "getAlbum", "id="+albumID)
	if resp4 == nil || resp4["status"] != "ok" {
		t.Fatalf("getAlbum after unstar failed: %v", resp4)
	}
	albumObj4, _ := resp4["album"].(map[string]any)
	songs4, _ := albumObj4["song"].([]any)
	if len(songs4) == 0 {
		t.Fatalf("no songs in album response after unstar")
	}
	songObj4, _ := songs4[0].(map[string]any)
	if _, hasStarred4 := songObj4["starred"]; hasStarred4 {
		t.Errorf("song 'starred' field should be absent after unstar: %v", songObj4)
	}
}

// TestRatingAppearsInGetSong verifies that setRating 4 causes getSong to return
// userRating==4, and that rating=0 removes it.
func TestRatingAppearsInGetSong(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	nodeID := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "rated-song.mp3", false)
	_, _, song := seed(t, ctx, h.q, userUUID, nodeID, "RateArt2", "RateAlb2", "RatedSong2")
	songID := encID("tr", db.UUIDString(song.ID))

	// Set rating to 4.
	resp := doGet(h, testAPIKey, "setRating", "id="+songID+"&rating=4")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("setRating failed: %v", resp)
	}

	// getSong should now carry userRating=4.
	resp2 := doGet(h, testAPIKey, "getSong", "id="+songID)
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("getSong failed: %v", resp2)
	}
	songObj, _ := resp2["song"].(map[string]any)
	ratingVal, hasRating := songObj["userRating"]
	if !hasRating {
		t.Errorf("song object missing 'userRating' field: %v", songObj)
	}
	if r, _ := ratingVal.(float64); int(r) != 4 {
		t.Errorf("userRating = %v, want 4", ratingVal)
	}

	// Remove rating (rating=0) and verify the field disappears.
	resp3 := doGet(h, testAPIKey, "setRating", "id="+songID+"&rating=0")
	if resp3 == nil || resp3["status"] != "ok" {
		t.Fatalf("setRating 0 failed: %v", resp3)
	}

	resp4 := doGet(h, testAPIKey, "getSong", "id="+songID)
	if resp4 == nil || resp4["status"] != "ok" {
		t.Fatalf("getSong after remove rating failed: %v", resp4)
	}
	songObj4, _ := resp4["song"].(map[string]any)
	if _, hasRating4 := songObj4["userRating"]; hasRating4 {
		t.Errorf("userRating should be absent after rating=0: %v", songObj4)
	}
}

// TestStarredAlbumInList verifies that starring an album makes the `starred` field
// appear on that album object in a getAlbumList2?type=newest response.
func TestStarredAlbumInList(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	nodeID := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "list-album-song.mp3", false)
	_, album, _ := seed(t, ctx, h.q, userUUID, nodeID, "ListArt", "ListAlb", "ListSong")
	albumID := encID("al", db.UUIDString(album.ID))

	// Star the album.
	resp := doGet(h, testAPIKey, "star", "albumId="+albumID)
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("star album failed: %v", resp)
	}

	// getAlbumList2?type=newest should include the album with `starred` set.
	rec := doGetRaw(h, testAPIKey, "getAlbumList2", "type=newest")
	albums := albumsFromResp(t, rec, "albumList2")

	found := false
	for _, a := range albums {
		am, _ := a.(map[string]any)
		if am["id"] == albumID {
			found = true
			starredVal, hasStarred := am["starred"]
			if !hasStarred {
				t.Errorf("starred album missing 'starred' field in getAlbumList2: %v", am)
			}
			if s, _ := starredVal.(string); s == "" {
				t.Errorf("starred field is present but empty in getAlbumList2: %v", am)
			}
		}
	}
	if !found {
		t.Errorf("starred album %q not found in getAlbumList2 response: %v", albumID, albums)
	}
}
