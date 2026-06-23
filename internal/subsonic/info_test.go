package subsonic

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

func TestGetArtistInfo2Minimal(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a.mp3", false)
	artist, _, _ := seed(t, ctx, h.q, userAID, n, "Radiohead", "OK Computer", "Airbag")
	id := encID("ar", db.UUIDString(artist.ID))
	resp := doGet(h, testAPIKey, "getArtistInfo2", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	if _, ok := resp["artistInfo2"].(map[string]any); !ok {
		t.Errorf("no artistInfo2 object: %v", resp)
	}
}

func TestGetTopSongs(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a.mp3", false)
	seed(t, ctx, h.q, userAID, n, "Radiohead", "OK Computer", "Airbag")
	resp := doGet(h, testAPIKey, "getTopSongs", "artist=Radiohead")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	ts, _ := resp["topSongs"].(map[string]any)
	if ts == nil {
		t.Fatalf("no topSongs: %v", resp)
	}
	songs, _ := ts["song"].([]any)
	if len(songs) != 1 {
		t.Errorf("topSongs count=%d, want 1", len(songs))
	}
}

func TestGetAlbumInfoMinimal(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a.mp3", false)
	_, album, _ := seed(t, ctx, h.q, userAID, n, "Radiohead", "OK Computer", "Airbag")
	id := encID("al", db.UUIDString(album.ID))
	resp := doGet(h, testAPIKey, "getAlbumInfo", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	if _, ok := resp["albumInfo"].(map[string]any); !ok {
		t.Errorf("no albumInfo object: %v", resp)
	}
}

func TestGetSimilarSongs2(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a.mp3", false)
	artist, _, _ := seed(t, ctx, h.q, userAID, n, "Radiohead", "OK Computer", "Airbag")
	id := encID("ar", db.UUIDString(artist.ID))
	resp := doGet(h, testAPIKey, "getSimilarSongs2", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	ss, _ := resp["similarSongs2"].(map[string]any)
	if ss == nil {
		t.Fatalf("no similarSongs2: %v", resp)
	}
	songs, _ := ss["song"].([]any)
	if len(songs) < 1 {
		t.Errorf("similarSongs2 returned no songs: %v", ss)
	}
}

func TestGetSimilarSongsGenreFill(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)

	// Seed Radiohead / OK Computer / Airbag for user A.
	nA := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "airbag.mp3", false)
	radioheadArtist, _, airbag := seed(t, ctx, h.q, userAID, nA, "Radiohead", "OK Computer", "Airbag")

	// Give "Airbag" the genre "Alternative" via UpsertSong re-upsert.
	_, err := h.q.UpsertSong(ctx, db.UpsertSongParams{
		UserID:   userAID,
		AlbumID:  airbag.AlbumID,
		ArtistID: airbag.ArtistID,
		NodeID:   airbag.NodeID,
		Title:    airbag.Title,
		Duration: pgtype.Int4{Int32: 180, Valid: true},
		Track:    pgtype.Int4{Int32: 1, Valid: true},
		Suffix:   pgtype.Text{String: "mp3", Valid: true},
		Genre:    pgtype.Text{String: "Alternative", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSong(genre): %v", err)
	}

	// Seed Muse / Absolution / Time Is Running Out for user A, same genre.
	nB := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "tiro.mp3", false)
	_, _, tiro := seed(t, ctx, h.q, userAID, nB, "Muse", "Absolution", "Time Is Running Out")
	_, err = h.q.UpsertSong(ctx, db.UpsertSongParams{
		UserID:   userAID,
		AlbumID:  tiro.AlbumID,
		ArtistID: tiro.ArtistID,
		NodeID:   tiro.NodeID,
		Title:    tiro.Title,
		Duration: pgtype.Int4{Int32: 215, Valid: true},
		Track:    pgtype.Int4{Int32: 2, Valid: true},
		Suffix:   pgtype.Text{String: "mp3", Valid: true},
		Genre:    pgtype.Text{String: "Alternative", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSong(genre Muse): %v", err)
	}

	// Request getSimilarSongs seeded from the Radiohead artist with count=10.
	id := encID("ar", db.UUIDString(radioheadArtist.ID))
	resp := doGet(h, testAPIKey, "getSimilarSongs", "id="+id+"&count=10")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok; full resp: %v", resp["status"], resp)
	}
	ss, _ := resp["similarSongs"].(map[string]any)
	if ss == nil {
		t.Fatalf("no similarSongs in response: %v", resp)
	}

	// Collect returned song titles.
	titles := map[string]bool{}
	for _, s := range ss["song"].([]any) {
		if sm, ok := s.(map[string]any); ok {
			if title, ok := sm["title"].(string); ok {
				titles[title] = true
			}
		}
	}

	// Genre-fill should have pulled in the Muse track (different artist, same genre).
	// Note: the seed is an artist (no genre), so genre comes from the song UpsertSong above.
	// The artist-seed path resolves genre as empty (pgtype.Text{Valid:false}), so genre-fill
	// won't fire from an artist seed — we test via a song seed instead.
	songID := encID("tr", db.UUIDString(airbag.ID))
	resp2 := doGet(h, testAPIKey, "getSimilarSongs", "id="+songID+"&count=10")
	if resp2["status"] != "ok" {
		t.Fatalf("song-seed status=%v, want ok; full resp: %v", resp2["status"], resp2)
	}
	ss2, _ := resp2["similarSongs"].(map[string]any)
	if ss2 == nil {
		t.Fatalf("no similarSongs (song seed): %v", resp2)
	}
	titles2 := map[string]bool{}
	for _, s := range ss2["song"].([]any) {
		if sm, ok := s.(map[string]any); ok {
			if title, ok := sm["title"].(string); ok {
				titles2[title] = true
			}
		}
	}
	if !titles2["Time Is Running Out"] {
		t.Errorf("genre-fill did not include Muse track; got titles: %v", titles2)
	}
}
