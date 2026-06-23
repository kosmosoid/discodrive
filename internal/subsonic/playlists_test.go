package subsonic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
	"discodrive/internal/secret"
)

// makeUserB creates a second music-enabled user in the same tenant as userA.
// Returns userB's UUID and apiKey.
func makeUserB(t *testing.T, ctx context.Context, h *Handler, userA db.User) (pgtype.UUID, string) {
	t.Helper()

	uB, err := h.q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     userA.TenantID,
		Email:        "b-pltest@x.test",
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
	ct, err := cipher.Encrypt("passB")
	if err != nil {
		t.Fatalf("cipher.Encrypt(B): %v", err)
	}

	if _, err = h.q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  uB.ID,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertMusicSettings(B): %v", err)
	}
	const apikeyB = "apikeyB-pltest"
	if err = h.q.SetMusicCredentials(ctx, db.SetMusicCredentialsParams{
		UserID:         uB.ID,
		PasswordCipher: pgtype.Text{String: ct, Valid: true},
		ApiKey:         pgtype.Text{String: apikeyB, Valid: true},
	}); err != nil {
		t.Fatalf("SetMusicCredentials(B): %v", err)
	}

	return uB.ID, apikeyB
}

// subsonicURL builds an authenticated GET URL for the test handler.
func subsonicURL(endpoint, apiKey, extra string) string {
	u := "/rest/" + endpoint + "?apiKey=" + apiKey + "&f=json&c=test&v=1.16.1"
	if extra != "" {
		u += "&" + extra
	}
	return u
}

// doReq fires a request at the handler and returns the parsed subsonic-response map.
func doReq(h *Handler, req *http.Request) map[string]any {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return subsonicResponse(m)
}

// doJSON performs a GET and returns the subsonic-response map.
func doJSON(h *Handler, target string) map[string]any {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return doReq(h, req)
}

// TestPlaylistCreateAndGet verifies basic create→get round-trip with two songs in order.
func TestPlaylistCreateAndGet(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "pl1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "pl2.mp3", false)
	_, _, song1 := seed(t, ctx, h.q, userUUID, n1, "Artist A", "Album A", "Song One")
	_, _, song2 := seed(t, ctx, h.q, userUUID, n2, "Artist A", "Album A", "Song Two")

	sid1 := encID("tr", db.UUIDString(song1.ID))
	sid2 := encID("tr", db.UUIDString(song2.ID))
	target := subsonicURL("createPlaylist", testAPIKey,
		"name=TestPL&songId="+sid1+"&songId="+sid2)

	resp := doJSON(h, target)
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("createPlaylist failed: %v", resp)
	}

	pl, _ := resp["playlist"].(map[string]any)
	if pl == nil {
		t.Fatalf("no playlist in response: %v", resp)
	}
	plID, _ := pl["id"].(string)
	if plID == "" {
		t.Fatalf("playlist id missing: %v", pl)
	}

	// getPlaylist should return the same songs in order.
	resp2 := doJSON(h, subsonicURL("getPlaylist", testAPIKey, "id="+plID))
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("getPlaylist failed: %v", resp2)
	}
	pl2, _ := resp2["playlist"].(map[string]any)
	entries, _ := pl2["entry"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 songs, got %d", len(entries))
	}
	first, _ := entries[0].(map[string]any)
	second, _ := entries[1].(map[string]any)
	if first["id"] != sid1 {
		t.Errorf("first song id = %v, want %v", first["id"], sid1)
	}
	if second["id"] != sid2 {
		t.Errorf("second song id = %v, want %v", second["id"], sid2)
	}
}

// TestPlaylistIsolation verifies user B cannot see or access user A's playlist.
func TestPlaylistIsolation(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userA, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	_, apikeyB := makeUserB(t, ctx, h, userA)

	// A creates a playlist.
	nA := makeNode(t, ctx, h.q, userA.ID, pgtype.UUID{}, "isol.mp3", false)
	seed(t, ctx, h.q, userA.ID, nA, "Art", "Alb", "Song")

	resp := doJSON(h, subsonicURL("createPlaylist", testAPIKey, "name=APrivate"))
	pl, _ := resp["playlist"].(map[string]any)
	plID, _ := pl["id"].(string)
	if plID == "" {
		t.Fatalf("A's playlist not created: %v", resp)
	}

	// B's getPlaylists should NOT list A's playlist.
	respB := doJSON(h, subsonicURL("getPlaylists", apikeyB, ""))
	plsWrapper, _ := respB["playlists"].(map[string]any)
	if plsWrapper != nil {
		pls, _ := plsWrapper["playlist"].([]any)
		for _, item := range pls {
			pl, _ := item.(map[string]any)
			if pl["id"] == plID {
				t.Errorf("B can see A's playlist in getPlaylists")
			}
		}
	}

	// B's getPlaylist for A's id should return not-found.
	respB2 := doJSON(h, subsonicURL("getPlaylist", apikeyB, "id="+plID))
	if respB2["status"] != "failed" {
		t.Errorf("B getPlaylist A's id: expected failed, got %v", respB2["status"])
	}
	errObj, _ := respB2["error"].(map[string]any)
	if code, _ := errObj["code"].(float64); int(code) != ErrNotFound {
		t.Errorf("error.code = %v, want %d", errObj["code"], ErrNotFound)
	}

	// B's deletePlaylist on A's id should fail silently but A's playlist should survive.
	doJSON(h, subsonicURL("deletePlaylist", apikeyB, "id="+plID))

	// A can still get her playlist.
	respA := doJSON(h, subsonicURL("getPlaylist", testAPIKey, "id="+plID))
	if respA["status"] != "ok" {
		t.Errorf("A's playlist was deleted by B: %v", respA)
	}
}

// TestPlaylistUpdate verifies add/remove/rename are all reflected.
func TestPlaylistUpdate(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "upd1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "upd2.mp3", false)
	n3 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "upd3.mp3", false)
	_, _, song1 := seed(t, ctx, h.q, userUUID, n1, "Ar", "Al", "S1")
	_, _, song2 := seed(t, ctx, h.q, userUUID, n2, "Ar", "Al", "S2")
	_, _, song3 := seed(t, ctx, h.q, userUUID, n3, "Ar", "Al", "S3")

	sid1 := encID("tr", db.UUIDString(song1.ID))
	sid2 := encID("tr", db.UUIDString(song2.ID))
	sid3 := encID("tr", db.UUIDString(song3.ID))

	// Create playlist with song1 and song2.
	resp := doJSON(h, subsonicURL("createPlaylist", testAPIKey,
		"name=Before&songId="+sid1+"&songId="+sid2))
	pl, _ := resp["playlist"].(map[string]any)
	plID, _ := pl["id"].(string)
	if plID == "" {
		t.Fatalf("createPlaylist failed: %v", resp)
	}
	encPlID := url.QueryEscape(plID)

	// Update: rename, remove index 0 (song1), add song3.
	updURL := subsonicURL("updatePlaylist", testAPIKey,
		"playlistId="+encPlID+"&name=After&songIndexToRemove=0&songIdToAdd="+sid3)
	resp2 := doJSON(h, updURL)
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("updatePlaylist failed: %v", resp2)
	}

	// Verify: getPlaylist should show [song2, song3] named "After".
	resp3 := doJSON(h, subsonicURL("getPlaylist", testAPIKey, "id="+encPlID))
	if resp3 == nil || resp3["status"] != "ok" {
		t.Fatalf("getPlaylist after update failed: %v", resp3)
	}
	pl3, _ := resp3["playlist"].(map[string]any)
	if pl3["name"] != "After" {
		t.Errorf("name = %v, want After", pl3["name"])
	}
	entries, _ := pl3["entry"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 songs after update, got %d: %v", len(entries), pl3)
	}
	e0, _ := entries[0].(map[string]any)
	e1, _ := entries[1].(map[string]any)
	if e0["id"] != sid2 {
		t.Errorf("entries[0] = %v, want %v", e0["id"], sid2)
	}
	if e1["id"] != sid3 {
		t.Errorf("entries[1] = %v, want %v", e1["id"], sid3)
	}
}

// TestCreatePlaylistViaPost exercises the formPost extension: POST with URL-encoded body,
// including multiple songId params and apiKey in the body.
func TestCreatePlaylistViaPost(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "post1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "post2.mp3", false)
	_, _, song1 := seed(t, ctx, h.q, userUUID, n1, "PostAr", "PostAl", "PostS1")
	_, _, song2 := seed(t, ctx, h.q, userUUID, n2, "PostAr", "PostAl", "PostS2")

	sid1 := encID("tr", db.UUIDString(song1.ID))
	sid2 := encID("tr", db.UUIDString(song2.ID))

	// Build a URL-encoded POST body (formPost extension).
	form := url.Values{}
	form.Set("apiKey", testAPIKey)
	form.Set("f", "json")
	form.Set("c", "test")
	form.Set("v", "1.16.1")
	form.Set("name", "Big")
	form.Add("songId", sid1)
	form.Add("songId", sid2)

	req := httptest.NewRequest(http.MethodPost, "/rest/createPlaylist", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp := doReq(h, req)
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("POST createPlaylist failed: %v", resp)
	}
	pl, _ := resp["playlist"].(map[string]any)
	entries, _ := pl["entry"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 songs from POST, got %d", len(entries))
	}
	e0, _ := entries[0].(map[string]any)
	e1, _ := entries[1].(map[string]any)
	if e0["id"] != sid1 {
		t.Errorf("entries[0].id = %v, want %v", e0["id"], sid1)
	}
	if e1["id"] != sid2 {
		t.Errorf("entries[1].id = %v, want %v", e1["id"], sid2)
	}
}
