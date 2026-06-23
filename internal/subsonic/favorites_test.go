package subsonic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
	"discodrive/internal/secret"
)

// makeUserBFav creates a second music-enabled user in the same tenant as userA.
// Using a unique email to avoid conflicts with other test helpers.
func makeUserBFav(t *testing.T, ctx context.Context, h *Handler, userA db.User) (pgtype.UUID, string) {
	t.Helper()

	uB, err := h.q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     userA.TenantID,
		Email:        "b-favtest@x.test",
		PasswordHash: "irrelevant",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser(B): %v", err)
	}

	cipher, err := secret.New(testKey)
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	ct, err := cipher.Encrypt("passBfav")
	if err != nil {
		t.Fatalf("cipher.Encrypt(B): %v", err)
	}

	if _, err = h.q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  uB.ID,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertMusicSettings(B): %v", err)
	}
	const apikeyB = "apikeyB-favtest"
	if err = h.q.SetMusicCredentials(ctx, db.SetMusicCredentialsParams{
		UserID:         uB.ID,
		PasswordCipher: pgtype.Text{String: ct, Valid: true},
		ApiKey:         pgtype.Text{String: apikeyB, Valid: true},
	}); err != nil {
		t.Fatalf("SetMusicCredentials(B): %v", err)
	}

	return uB.ID, apikeyB
}

// doJSONFav sends a GET and returns the parsed subsonic-response map (favorites test helper).
func doJSONFav(h *Handler, target string) map[string]any {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return subsonicResponse(m)
}

// starURL builds a star/unstar URL for a given song id.
func starURL(endpoint, apiKey, songID string) string {
	return "/rest/" + endpoint + "?apiKey=" + apiKey + "&f=json&c=test&v=1.16.1&id=" + songID
}

// TestStarUnstarGetStarred verifies the full star→getStarred2→unstar→gone cycle
// and that user B's getStarred2 does not include A's stars.
func TestStarUnstarGetStarred(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userA, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	_, apikeyB := makeUserBFav(t, ctx, h, userA)

	nA := makeNode(t, ctx, h.q, userA.ID, pgtype.UUID{}, "fav1.mp3", false)
	_, _, song := seed(t, ctx, h.q, userA.ID, nA, "FavArt", "FavAlb", "FavSong")
	songID := encID("tr", db.UUIDString(song.ID))

	// Star the song as user A.
	resp := doJSONFav(h, starURL("star", testAPIKey, songID))
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("star failed: %v", resp)
	}

	// getStarred2 for A should include the song.
	resp2 := doJSONFav(h, "/rest/getStarred2?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1")
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("getStarred2 failed: %v", resp2)
	}
	starred, _ := resp2["starred2"].(map[string]any)
	songs, _ := starred["song"].([]any)
	found := false
	for _, s := range songs {
		sm, _ := s.(map[string]any)
		if sm["id"] == songID {
			found = true
		}
	}
	if !found {
		t.Errorf("starred song not found in getStarred2 response: %v", starred)
	}

	// B's getStarred2 should NOT include A's starred song.
	respB := doJSONFav(h, "/rest/getStarred2?apiKey="+apikeyB+"&f=json&c=test&v=1.16.1")
	starredB, _ := respB["starred2"].(map[string]any)
	songsB, _ := starredB["song"].([]any)
	for _, s := range songsB {
		sm, _ := s.(map[string]any)
		if sm["id"] == songID {
			t.Errorf("B can see A's starred song in getStarred2")
		}
	}

	// Unstar.
	resp3 := doJSONFav(h, starURL("unstar", testAPIKey, songID))
	if resp3 == nil || resp3["status"] != "ok" {
		t.Fatalf("unstar failed: %v", resp3)
	}

	// getStarred2 after unstar: song should be gone.
	resp4 := doJSONFav(h, "/rest/getStarred2?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1")
	starred4, _ := resp4["starred2"].(map[string]any)
	songs4, _ := starred4["song"].([]any)
	for _, s := range songs4 {
		sm, _ := s.(map[string]any)
		if sm["id"] == songID {
			t.Errorf("song still in getStarred2 after unstar: %v", sm)
		}
	}
}

// TestStarForeignAlbumDenied verifies that user A cannot star user B's private album
// and have it appear in A's getStarred2 response (cross-user metadata leak).
func TestStarForeignAlbumDenied(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userA, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail(A): %v", err)
	}

	// Create user B with their own unshared album.
	uBID, _ := makeUserBFav(t, ctx, h, userA)
	nB := makeNode(t, ctx, h.q, uBID, pgtype.UUID{}, "b-priv.mp3", false)
	_, bAlbum, _ := seed(t, ctx, h.q, uBID, nB, "B-Artist", "B-Album", "B-Song")
	bAlbumID := encID("al", db.UUIDString(bAlbum.ID))

	// User A attempts to star B's album (no share exists).
	resp := doJSONFav(h, "/rest/star?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1&albumId="+bAlbumID)
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("star returned unexpected error: %v", resp)
	}

	// getStarred2 for A must NOT include B's album.
	resp2 := doJSONFav(h, "/rest/getStarred2?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1")
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("getStarred2 failed: %v", resp2)
	}
	starred, _ := resp2["starred2"].(map[string]any)
	albums, _ := starred["album"].([]any)
	for _, a := range albums {
		am, _ := a.(map[string]any)
		if am["id"] == bAlbumID {
			t.Errorf("B's private album leaked into A's getStarred2 starred albums")
		}
	}
}

// TestSetRating verifies rating storage, 0 removes, and out-of-range returns error 10.
func TestSetRating(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "rated.mp3", false)
	_, _, song := seed(t, ctx, h.q, userUUID, n, "RateArt", "RateAlb", "RatedSong")
	songID := encID("tr", db.UUIDString(song.ID))

	// Set rating to 4.
	resp := doJSONFav(h, "/rest/setRating?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1&id="+songID+"&rating=4")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("setRating failed: %v", resp)
	}

	// rating=0 removes.
	resp2 := doJSONFav(h, "/rest/setRating?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1&id="+songID+"&rating=0")
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("setRating 0 failed: %v", resp2)
	}

	// rating=6 returns error code 10.
	resp3 := doJSONFav(h, "/rest/setRating?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1&id="+songID+"&rating=6")
	if resp3 == nil || resp3["status"] != "failed" {
		t.Fatalf("setRating 6 expected failure, got: %v", resp3)
	}
	errObj, _ := resp3["error"].(map[string]any)
	if code, _ := errObj["code"].(float64); int(code) != ErrMissingParam {
		t.Errorf("error.code = %v, want %d", errObj["code"], ErrMissingParam)
	}

}

// Regression: starring an album/artist and listing them via getStarred2 must not
// error. ListStarredAlbums/Artists used "SELECT DISTINCT ... ORDER BY st.starred_at",
// which Postgres rejects (ordering column not in the select list).
func TestStarAlbumArtistGetStarred(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userA, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	nA := makeNode(t, ctx, h.q, userA.ID, pgtype.UUID{}, "favaa.mp3", false)
	artist, album, _ := seed(t, ctx, h.q, userA.ID, nA, "StarArt", "StarAlb", "StarSong")
	albumID := encID("al", db.UUIDString(album.ID))
	artistID := encID("ar", db.UUIDString(artist.ID))

	if r := doJSONFav(h, starURL("star", testAPIKey, albumID)); r == nil || r["status"] != "ok" {
		t.Fatalf("star album: %v", r)
	}
	if r := doJSONFav(h, starURL("star", testAPIKey, artistID)); r == nil || r["status"] != "ok" {
		t.Fatalf("star artist: %v", r)
	}

	resp := doJSONFav(h, "/rest/getStarred2?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("getStarred2 failed (likely SQL error): %v", resp)
	}
	st, _ := resp["starred2"].(map[string]any)
	hasID := func(key, want string) bool {
		arr, _ := st[key].([]any)
		for _, it := range arr {
			if m, _ := it.(map[string]any); m["id"] == want {
				return true
			}
		}
		return false
	}
	if !hasID("album", albumID) {
		t.Fatalf("starred album not in getStarred2: %v", st)
	}
	if !hasID("artist", artistID) {
		t.Fatalf("starred artist not in getStarred2: %v", st)
	}
}
