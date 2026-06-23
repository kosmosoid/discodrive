package subsonic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// --- helpers ---

// seedWithGenre creates an artist, album (with genre and year) and one song owned by userID.
func seedWithGenre(t *testing.T, ctx context.Context, q *db.Queries,
	userID, nodeID pgtype.UUID, artistName, albumName, songTitle, genre string, year int32,
) (db.Artist, db.Album, db.Song) {
	t.Helper()

	artist, err := q.UpsertArtist(ctx, db.UpsertArtistParams{
		UserID: userID, Name: artistName, SortName: artistName,
	})
	if err != nil {
		t.Fatalf("UpsertArtist(%q): %v", artistName, err)
	}

	album, err := q.UpsertAlbum(ctx, db.UpsertAlbumParams{
		UserID:   userID,
		ArtistID: artist.ID,
		Name:     albumName,
		Year:     pgtype.Int4{Int32: year, Valid: true},
		Genre:    pgtype.Text{String: genre, Valid: genre != ""},
	})
	if err != nil {
		t.Fatalf("UpsertAlbum(%q): %v", albumName, err)
	}

	song, err := q.UpsertSong(ctx, db.UpsertSongParams{
		UserID:   userID,
		AlbumID:  album.ID,
		ArtistID: artist.ID,
		NodeID:   nodeID,
		Title:    songTitle,
		Duration: pgtype.Int4{Int32: 200, Valid: true},
		Genre:    pgtype.Text{String: genre, Valid: genre != ""},
	})
	if err != nil {
		t.Fatalf("UpsertSong(%q): %v", songTitle, err)
	}
	if err := q.RefreshAlbumSongCount(ctx, album.ID); err != nil {
		t.Fatalf("RefreshAlbumSongCount: %v", err)
	}
	return artist, album, song
}

// doGetRaw sends a Subsonic JSON request and returns the raw ResponseRecorder.
func doGetRaw(h *Handler, apiKey, endpoint, queryExtra string) *httptest.ResponseRecorder {
	target := "/rest/" + endpoint + "?apiKey=" + apiKey + "&f=json&c=test&v=1.16.1"
	if queryExtra != "" {
		target += "&" + queryExtra
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// albumsFromResp extracts the album slice from a getAlbumList2/getAlbumList response.
func albumsFromResp(t *testing.T, rec *httptest.ResponseRecorder, wrapKey string) []any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v — body: %s", err, rec.Body.String())
	}
	inner, _ := m["subsonic-response"].(map[string]any)
	if inner == nil {
		t.Fatalf("no subsonic-response: %s", rec.Body.String())
	}
	if inner["status"] != "ok" {
		t.Fatalf("status=%v: %s", inner["status"], rec.Body.String())
	}
	wrapper, _ := inner[wrapKey].(map[string]any)
	if wrapper == nil {
		t.Fatalf("no %q key in response: %v", wrapKey, inner)
	}
	albums, _ := wrapper["album"].([]any)
	return albums
}

// --- Tests ---

func TestGetAlbumList2Newest(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	// Seed three albums owned by the test user (different creation order).
	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "s1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "s2.mp3", false)
	n3 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "s3.mp3", false)

	seedWithGenre(t, ctx, h.q, userUUID, n1, "Artist A", "Album Alpha", "Song A", "Rock", 2020)
	seedWithGenre(t, ctx, h.q, userUUID, n2, "Artist B", "Album Beta", "Song B", "Pop", 2021)
	seedWithGenre(t, ctx, h.q, userUUID, n3, "Artist C", "Album Gamma", "Song C", "Rock", 2022)

	rec := doGetRaw(h, testAPIKey, "getAlbumList2", "type=newest&size=2")
	albums := albumsFromResp(t, rec, "albumList2")

	if len(albums) != 2 {
		t.Fatalf("expected 2 albums (size=2), got %d", len(albums))
	}

	// The first album should be the most recently inserted one.
	first, _ := albums[0].(map[string]any)
	if first["name"] != "Album Gamma" {
		t.Errorf("expected newest album first, got %q", first["name"])
	}

	// Verify the id is encoded properly.
	id, _ := first["id"].(string)
	kind, _, ok := decID(id)
	if !ok || kind != "al" {
		t.Errorf("album id %q should decode to kind=al", id)
	}
}

func TestGetAlbumList2ByGenre(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "g1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "g2.mp3", false)
	n3 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "g3.mp3", false)

	seedWithGenre(t, ctx, h.q, userUUID, n1, "Artist X", "Jazz Album", "Song X", "Jazz", 2019)
	seedWithGenre(t, ctx, h.q, userUUID, n2, "Artist Y", "Rock Album 1", "Song Y", "Rock", 2020)
	seedWithGenre(t, ctx, h.q, userUUID, n3, "Artist Z", "Rock Album 2", "Song Z", "Rock", 2021)

	rec := doGetRaw(h, testAPIKey, "getAlbumList2", "type=byGenre&genre=Rock")
	albums := albumsFromResp(t, rec, "albumList2")

	if len(albums) != 2 {
		t.Fatalf("expected 2 Rock albums, got %d: %s", len(albums), rec.Body.String())
	}
	for _, a := range albums {
		al, _ := a.(map[string]any)
		if g, _ := al["genre"].(string); g != "Rock" {
			t.Errorf("album genre = %q, want Rock", g)
		}
	}
}

func TestGetAlbumListLegacy(t *testing.T) {
	// getAlbumList (legacy) should return the same data under the "albumList" key.
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "la1.mp3", false)
	seedWithGenre(t, ctx, h.q, userUUID, n1, "Legacy Artist", "Legacy Album", "Legacy Song", "Blues", 2000)

	rec := doGetRaw(h, testAPIKey, "getAlbumList", "type=newest")
	albums := albumsFromResp(t, rec, "albumList")
	if len(albums) == 0 {
		t.Fatalf("expected albums in getAlbumList response, got none: %s", rec.Body.String())
	}
}

func TestGetGenres(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "ge1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "ge2.mp3", false)
	n3 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "ge3.mp3", false)

	seedWithGenre(t, ctx, h.q, userUUID, n1, "GA", "GA1", "Song A", "Metal", 2020)
	seedWithGenre(t, ctx, h.q, userUUID, n2, "GB", "GB1", "Song B", "Metal", 2021)
	seedWithGenre(t, ctx, h.q, userUUID, n3, "GC", "GC1", "Song C", "Reggae", 2022)

	rec := doGetRaw(h, testAPIKey, "getGenres", "")
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	inner, _ := m["subsonic-response"].(map[string]any)
	if inner == nil || inner["status"] != "ok" {
		t.Fatalf("bad response: %s", rec.Body.String())
	}

	genresWrapper, _ := inner["genres"].(map[string]any)
	if genresWrapper == nil {
		t.Fatalf("no genres key: %v", inner)
	}
	genreList, _ := genresWrapper["genre"].([]any)

	genreMap := map[string]map[string]any{}
	for _, g := range genreList {
		gm, _ := g.(map[string]any)
		if v, ok := gm["value"].(string); ok {
			genreMap[v] = gm
		}
	}

	if _, ok := genreMap["Metal"]; !ok {
		t.Errorf("Metal genre missing from response: %v", genreMap)
	}
	if _, ok := genreMap["Reggae"]; !ok {
		t.Errorf("Reggae genre missing from response: %v", genreMap)
	}

	// Metal has 2 songs and 2 albums.
	metal := genreMap["Metal"]
	if sc, _ := metal["songCount"].(float64); int(sc) != 2 {
		t.Errorf("Metal.songCount = %v, want 2", metal["songCount"])
	}
	if ac, _ := metal["albumCount"].(float64); int(ac) != 2 {
		t.Errorf("Metal.albumCount = %v, want 2", metal["albumCount"])
	}
}

// minimalJPEG is the smallest valid JFIF-encoded JPEG (1×1 white pixel).
var minimalJPEG = []byte{
	0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xdb, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0a, 0x0c, 0x14, 0x0d, 0x0c, 0x0b, 0x0b, 0x0c, 0x19, 0x12,
	0x13, 0x0f, 0x14, 0x1d, 0x1a, 0x1f, 0x1e, 0x1d, 0x1a, 0x1c, 0x1c, 0x20,
	0x24, 0x2e, 0x27, 0x20, 0x22, 0x2c, 0x23, 0x1c, 0x1c, 0x28, 0x37, 0x29,
	0x2c, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1f, 0x27, 0x39, 0x3d, 0x38, 0x32,
	0x3c, 0x2e, 0x33, 0x34, 0x32, 0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xff, 0xc4, 0x00, 0x1f, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0a, 0x0b, 0xff, 0xc4, 0x00, 0xb5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7d,
	0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
	0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xa1, 0x08,
	0x23, 0x42, 0xb1, 0xc1, 0x15, 0x52, 0xd1, 0xf0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0a, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x25, 0x26, 0x27, 0x28,
	0x29, 0x2a, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x43, 0x44, 0x45,
	0x46, 0x47, 0x48, 0x49, 0x4a, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
	0x5a, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7a, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
	0x8a, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0xa2, 0xa3, 0xa4,
	0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7,
	0xb8, 0xb9, 0xba, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, 0xc8, 0xc9, 0xca,
	0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xe1, 0xe2, 0xe3,
	0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5,
	0xf6, 0xf7, 0xf8, 0xf9, 0xfa, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01, 0x00,
	0x00, 0x3f, 0x00, 0xfb, 0xd7, 0xff, 0xd9,
}

func TestGetCoverArtSiblingImage(t *testing.T) {
	// Set up a storage root with a real JPEG cover file on disk.
	storageRoot := t.TempDir()

	h, ctx := setupSubsonic(t)
	// Replace the handler with one that knows about storageRoot.
	h.storageRoot = storageRoot
	h.xaccel = false

	userUUID := mustUserID(t, ctx, h.q, testEmail)

	// Write the JPEG to the storage root.
	relPath := "covers/test-cover.jpg"
	absPath := filepath.Join(storageRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absPath, minimalJPEG, 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}

	// Create a node row for the cover image.
	coverNode, err := h.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   userUUID,
		Name:     "test-cover.jpg",
		IsDir:    false,
		DiskPath: pgtype.Text{String: relPath, Valid: true},
		Mime:     pgtype.Text{String: "image/jpeg", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode(cover): %v", err)
	}

	// Seed an album that points at this cover node.
	songNode := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "cover-song.mp3", false)
	_, album, _ := seed(t, ctx, h.q, userUUID, songNode, "Cover Artist", "Cover Album", "Cover Song")

	// Set the album's cover_art to the cover node's UUID string.
	if err := h.q.SetAlbumCover(ctx, db.SetAlbumCoverParams{
		ID:       album.ID,
		CoverArt: pgtype.Text{String: db.UUIDString(coverNode.ID), Valid: true},
	}); err != nil {
		t.Fatalf("SetAlbumCover: %v", err)
	}

	// getCoverArt?id=al-<uuid> → 200 + JPEG bytes.
	coverID := encID("al", db.UUIDString(album.ID))
	rec := doGetRaw(h, testAPIKey, "getCoverArt", "id="+coverID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if got := rec.Body.Bytes(); string(got) != string(minimalJPEG) {
		t.Errorf("body length=%d, want %d", len(got), len(minimalJPEG))
	}

	// Range request should return 206 Partial Content.
	rangeReq := httptest.NewRequest(http.MethodGet,
		"/rest/getCoverArt?apiKey="+testAPIKey+"&f=json&c=test&v=1.16.1&id="+coverID, nil)
	rangeReq.Header.Set("Range", "bytes=0-9")
	rangeRec := httptest.NewRecorder()
	h.ServeHTTP(rangeRec, rangeReq)
	if rangeRec.Code != http.StatusPartialContent {
		t.Errorf("Range request: expected 206, got %d", rangeRec.Code)
	}
	if rangeRec.Body.Len() != 10 {
		t.Errorf("Range body length = %d, want 10", rangeRec.Body.Len())
	}
}

// type=random must not error: Postgres rejects "SELECT DISTINCT ... ORDER BY random()",
// so the query dedupes in a derived table. Regression test for the live SQL error.
func TestGetAlbumList2Random(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "r1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "r2.mp3", false)
	seedWithGenre(t, ctx, h.q, userUUID, n1, "Artist A", "Album Alpha", "Song A", "Rock", 2020)
	seedWithGenre(t, ctx, h.q, userUUID, n2, "Artist B", "Album Beta", "Song B", "Pop", 2021)

	rec := doGetRaw(h, testAPIKey, "getAlbumList2", "type=random&size=10")
	albums := albumsFromResp(t, rec, "albumList2")
	if len(albums) != 2 {
		t.Fatalf("expected 2 random albums, got %d — body: %s", len(albums), rec.Body.String())
	}
}
