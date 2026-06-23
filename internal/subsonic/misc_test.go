package subsonic

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// TestGetRandomSongs verifies getRandomSongs returns at most size accessible songs.
func TestGetRandomSongs(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	// Seed 3 songs.
	for i, title := range []string{"Song A", "Song B", "Song C"} {
		n := makeNodeOwnedBy(t, ctx, h.q, userUUID, title+".mp3")
		_ = i
		seed(t, ctx, h.q, userUUID, n, "Random Artist", "Random Album", title)
	}

	resp := doGet(h, testAPIKey, "getRandomSongs", "size=2")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getRandomSongs failed: %v", resp)
	}

	wrapper, _ := resp["randomSongs"].(map[string]any)
	if wrapper == nil {
		t.Fatalf("no randomSongs key in response: %v", resp)
	}
	songs, _ := wrapper["song"].([]any)
	if len(songs) > 2 {
		t.Errorf("size=2 should return at most 2 songs, got %d", len(songs))
	}
	if len(songs) == 0 {
		t.Error("expected at least 1 song in response")
	}

	// Each returned song should have "id" with "tr" prefix.
	for _, s := range songs {
		sm, ok := s.(map[string]any)
		if !ok {
			t.Errorf("song is not a map: %v", s)
			continue
		}
		id, _ := sm["id"].(string)
		kind, _, ok2 := decID(id)
		if !ok2 || kind != "tr" {
			t.Errorf("song id %q should decode to kind=tr", id)
		}
	}
}

// TestGetRandomSongsFilterGenre verifies the genre filter.
func TestGetRandomSongsFilterGenre(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNodeOwnedBy(t, ctx, h.q, userUUID, "rock.mp3")
	n2 := makeNodeOwnedBy(t, ctx, h.q, userUUID, "jazz.mp3")
	seedWithGenre(t, ctx, h.q, userUUID, n1, "Rock Artist", "Rock Album", "Rock Song", "Rock", 2020)
	seedWithGenre(t, ctx, h.q, userUUID, n2, "Jazz Artist", "Jazz Album", "Jazz Song", "Jazz", 2020)

	resp := doGet(h, testAPIKey, "getRandomSongs", "size=10&genre=Jazz")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getRandomSongs(genre=Jazz) failed: %v", resp)
	}

	wrapper, _ := resp["randomSongs"].(map[string]any)
	songs, _ := wrapper["song"].([]any)
	if len(songs) == 0 {
		t.Fatal("expected at least 1 Jazz song")
	}
	for _, s := range songs {
		sm, _ := s.(map[string]any)
		if g, _ := sm["genre"].(string); g != "Jazz" {
			t.Errorf("expected genre=Jazz, got %q", g)
		}
	}
}

// TestGetSongsByGenre verifies getSongsByGenre filters correctly.
func TestGetSongsByGenre(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNodeOwnedBy(t, ctx, h.q, userUUID, "metal1.mp3")
	n2 := makeNodeOwnedBy(t, ctx, h.q, userUUID, "pop1.mp3")
	seedWithGenre(t, ctx, h.q, userUUID, n1, "Metal Band", "Metal Album", "Metal Song", "Metal", 2020)
	seedWithGenre(t, ctx, h.q, userUUID, n2, "Pop Band", "Pop Album", "Pop Song", "Pop", 2021)

	resp := doGet(h, testAPIKey, "getSongsByGenre", "genre=Metal")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getSongsByGenre failed: %v", resp)
	}

	wrapper, _ := resp["songsByGenre"].(map[string]any)
	if wrapper == nil {
		t.Fatalf("no songsByGenre in response: %v", resp)
	}
	songs, _ := wrapper["song"].([]any)
	if len(songs) == 0 {
		t.Fatal("expected at least 1 Metal song")
	}
	for _, s := range songs {
		sm, _ := s.(map[string]any)
		if g, _ := sm["genre"].(string); g != "Metal" {
			t.Errorf("expected genre=Metal, got %q", g)
		}
	}
}

// TestGetSongsByGenreMissingParam verifies that omitting genre returns error code 10.
func TestGetSongsByGenreMissingParam(t *testing.T) {
	h, _ := setupSubsonic(t)

	resp := doGet(h, testAPIKey, "getSongsByGenre", "")
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp["status"] != "failed" {
		t.Errorf("expected failed status, got %v", resp["status"])
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("no error object in response: %v", resp)
	}
	if code, _ := errObj["code"].(float64); int(code) != ErrMissingParam {
		t.Errorf("error.code = %v, want %d", code, ErrMissingParam)
	}
}

// TestGetNowPlaying verifies getNowPlaying lists a recently scrobbled song.
func TestGetNowPlaying(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n := makeNodeOwnedBy(t, ctx, h.q, userUUID, "np.mp3")
	_, _, song := seed(t, ctx, h.q, userUUID, n, "NP Artist", "NP Album", "Now Playing Song")

	// Insert a play_history row directly.
	if err := h.q.InsertPlayHistory(ctx, db.InsertPlayHistoryParams{
		UserID: userUUID,
		SongID: song.ID,
	}); err != nil {
		t.Fatalf("InsertPlayHistory: %v", err)
	}

	resp := doGet(h, testAPIKey, "getNowPlaying", "")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getNowPlaying failed: %v", resp)
	}

	wrapper, _ := resp["nowPlaying"].(map[string]any)
	if wrapper == nil {
		t.Fatalf("no nowPlaying key in response: %v", resp)
	}
	entries, _ := wrapper["entry"].([]any)
	if len(entries) == 0 {
		t.Fatal("expected at least 1 now-playing entry")
	}

	entry, _ := entries[0].(map[string]any)
	if entry["title"] != "Now Playing Song" {
		t.Errorf("entry.title = %q, want 'Now Playing Song'", entry["title"])
	}
	// Verify extra fields.
	if _, ok := entry["minutesAgo"]; !ok {
		t.Error("entry should have minutesAgo field")
	}
	if _, ok := entry["playerId"]; !ok {
		t.Error("entry should have playerId field")
	}
	if _, ok := entry["username"]; !ok {
		t.Error("entry should have username field")
	}
}

// TestGetNowPlayingEmpty verifies an empty history returns an empty entries list (not an error).
func TestGetNowPlayingEmpty(t *testing.T) {
	h, _ := setupSubsonic(t)

	resp := doGet(h, testAPIKey, "getNowPlaying", "")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getNowPlaying empty failed: %v", resp)
	}

	wrapper, _ := resp["nowPlaying"].(map[string]any)
	if wrapper == nil {
		t.Fatalf("no nowPlaying key: %v", resp)
	}
	// Entries may be nil or empty slice — both are acceptable.
	// Just confirm no panic and ok status (already checked above).
}

// TestGetRandomSongsScoping verifies that random songs does not leak other users' private songs.
func TestGetRandomSongsScoping(t *testing.T) {
	h, ctx, userAID, userBID := setupTwo(t)

	nA := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a-random.mp3", false)
	nB := makeNode(t, ctx, h.q, userBID, pgtype.UUID{}, "b-random.mp3", false)
	seed(t, ctx, h.q, userAID, nA, "A-Artist", "A-Album", "A-Song")
	seed(t, ctx, h.q, userBID, nB, "B-Artist", "B-Album", "B-Song")

	// User A calls getRandomSongs — should only get their own songs.
	resp := doGet(h, testAPIKey, "getRandomSongs", "size=50")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getRandomSongs failed: %v", resp)
	}

	wrapper, _ := resp["randomSongs"].(map[string]any)
	songs, _ := wrapper["song"].([]any)
	for _, s := range songs {
		sm, _ := s.(map[string]any)
		if title, _ := sm["title"].(string); title == "B-Song" {
			t.Errorf("B-Song (private) should not appear in A's random songs")
		}
	}
}

// ensure makeNodeOwnedBy is used (it is in search_test.go too, but referenced here).
var _ = context.Background
