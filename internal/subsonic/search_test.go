package subsonic

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// collectSearchArtists extracts artist names from a searchResult3/searchResult2 response.
func collectSearchArtists(resp map[string]any, wrapKey string) []string {
	wrapper, _ := resp[wrapKey].(map[string]any)
	if wrapper == nil {
		return nil
	}
	artists, _ := wrapper["artist"].([]any)
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		if am, ok := a.(map[string]any); ok {
			if n, ok := am["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	return names
}

// collectSearchAlbums extracts album names from a searchResult3 response.
func collectSearchAlbums(resp map[string]any, wrapKey string) []string {
	wrapper, _ := resp[wrapKey].(map[string]any)
	if wrapper == nil {
		return nil
	}
	albums, _ := wrapper["album"].([]any)
	names := make([]string, 0, len(albums))
	for _, a := range albums {
		if am, ok := a.(map[string]any); ok {
			if n, ok := am["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	return names
}

// collectSearchSongs extracts song titles from a searchResult3 response.
func collectSearchSongs(resp map[string]any, wrapKey string) []string {
	wrapper, _ := resp[wrapKey].(map[string]any)
	if wrapper == nil {
		return nil
	}
	songs, _ := wrapper["song"].([]any)
	titles := make([]string, 0, len(songs))
	for _, s := range songs {
		if sm, ok := s.(map[string]any); ok {
			if t, ok := sm["title"].(string); ok {
				titles = append(titles, t)
			}
		}
	}
	return titles
}

// containsStr returns true if slice contains s.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// TestSearch3FindsByName seeds artist "Pink Floyd" / album "The Wall" / song,
// then verifies that query=floyd finds the artist and query=wall finds the album.
func TestSearch3FindsByName(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "comfortably.mp3", false)
	seed(t, ctx, h.q, userUUID, n, "Pink Floyd", "The Wall", "Comfortably Numb")

	// Artist search: query=floyd
	resp := doGet(h, testAPIKey, "search3", "query=floyd")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("search3 failed: %v", resp)
	}
	artists := collectSearchArtists(resp, "searchResult3")
	if !containsStr(artists, "Pink Floyd") {
		t.Errorf("expected 'Pink Floyd' in artist results, got: %v", artists)
	}

	// Album search: query=wall
	resp2 := doGet(h, testAPIKey, "search3", "query=wall")
	if resp2 == nil || resp2["status"] != "ok" {
		t.Fatalf("search3 failed: %v", resp2)
	}
	albums := collectSearchAlbums(resp2, "searchResult3")
	if !containsStr(albums, "The Wall") {
		t.Errorf("expected 'The Wall' in album results, got: %v", albums)
	}
}

// TestSearch3EmptyReturnsLibrary verifies that an empty query returns all accessible content.
func TestSearch3EmptyReturnsLibrary(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n1 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "s1.mp3", false)
	n2 := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "s2.mp3", false)
	seed(t, ctx, h.q, userUUID, n1, "Artist Alpha", "Album One", "Song One")
	seed(t, ctx, h.q, userUUID, n2, "Artist Beta", "Album Two", "Song Two")

	// Empty query — should return everything.
	resp := doGet(h, testAPIKey, "search3", "query=&artistCount=100&albumCount=100&songCount=100")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("search3 failed: %v", resp)
	}

	artists := collectSearchArtists(resp, "searchResult3")
	if !containsStr(artists, "Artist Alpha") || !containsStr(artists, "Artist Beta") {
		t.Errorf("empty query should return all artists, got: %v", artists)
	}

	albums := collectSearchAlbums(resp, "searchResult3")
	if !containsStr(albums, "Album One") || !containsStr(albums, "Album Two") {
		t.Errorf("empty query should return all albums, got: %v", albums)
	}

	songs := collectSearchSongs(resp, "searchResult3")
	if !containsStr(songs, "Song One") || !containsStr(songs, "Song Two") {
		t.Errorf("empty query should return all songs, got: %v", songs)
	}
}

// TestSearch3Scoping verifies a user cannot see unshared content owned by another user.
func TestSearch3Scoping(t *testing.T) {
	h, ctx, userAID, userBID := setupTwo(t)

	// User A has their own content.
	nA := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a.mp3", false)
	seed(t, ctx, h.q, userAID, nA, "A-Artist", "A-Album", "A-Song")

	// User B has private content (no share to A).
	nB := makeNode(t, ctx, h.q, userBID, pgtype.UUID{}, "b.mp3", false)
	seed(t, ctx, h.q, userBID, nB, "B-Private-Artist", "B-Private-Album", "B-Private-Song")

	// User A searches — should NOT see B's artist.
	resp := doGet(h, testAPIKey, "search3", "query=&artistCount=100")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("search3 failed: %v", resp)
	}

	artists := collectSearchArtists(resp, "searchResult3")
	if containsStr(artists, "B-Private-Artist") {
		t.Errorf("B-Private-Artist should not be visible to user A, but found in: %v", artists)
	}
	if !containsStr(artists, "A-Artist") {
		t.Errorf("A-Artist should be visible, got: %v", artists)
	}
}

// TestSearch2Wrapper verifies search2 uses "searchResult2" as the wrapper key.
func TestSearch2Wrapper(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	n := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, "track.mp3", false)
	seed(t, ctx, h.q, userUUID, n, "Radiohead", "OK Computer", "Karma Police")

	resp := doGet(h, testAPIKey, "search2", "query=radiohead")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("search2 failed: %v", resp)
	}

	// Key must be "searchResult2", not "searchResult3".
	if _, ok := resp["searchResult3"]; ok {
		t.Error("search2 must use searchResult2, not searchResult3")
	}
	artists := collectSearchArtists(resp, "searchResult2")
	if !containsStr(artists, "Radiohead") {
		t.Errorf("expected Radiohead in search2 results, got: %v", artists)
	}
}

// TestSearch3PaginationArtistCount verifies artistCount limits the number of returned artists.
func TestSearch3PaginationArtistCount(t *testing.T) {
	h, ctx := setupSubsonic(t)
	userUUID := mustUserID(t, ctx, h.q, testEmail)

	// Seed three artists.
	for _, name := range []string{"Band A", "Band B", "Band C"} {
		n := makeNode(t, ctx, h.q, userUUID, pgtype.UUID{}, name+".mp3", false)
		seed(t, ctx, h.q, userUUID, n, name, name+" Album", name+" Song")
	}

	// Request only 2 artists.
	resp := doGet(h, testAPIKey, "search3", "query=band&artistCount=2")
	if resp == nil || resp["status"] != "ok" {
		t.Fatalf("search3 failed: %v", resp)
	}

	artists := collectSearchArtists(resp, "searchResult3")
	if len(artists) > 2 {
		t.Errorf("artistCount=2 should return at most 2 artists, got %d", len(artists))
	}
}

// makeNodeOwnedBy is a helper that creates a node owned by ownerID (no parent).
func makeNodeOwnedBy(t *testing.T, ctx context.Context, q *db.Queries, ownerID pgtype.UUID, name string) pgtype.UUID {
	t.Helper()
	return makeNode(t, ctx, q, ownerID, pgtype.UUID{}, name, false)
}
