package subsonic

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

func childTitles(resp map[string]any) map[string]bool {
	out := map[string]bool{}
	dir, _ := resp["directory"].(map[string]any)
	if dir == nil {
		return out
	}
	children, _ := dir["child"].([]any)
	for _, ch := range children {
		c, _ := ch.(map[string]any)
		if tn, ok := c["title"].(string); ok {
			out[tn] = true
		}
	}
	return out
}

func TestGetMusicDirectoryRoot(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a1.mp3", false)
	seed(t, ctx, h.q, userAID, n, "Arctic Monkeys", "AM", "Do I Wanna Know?")

	resp := doGet(h, testAPIKey, "getMusicDirectory", "id=0")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	if !childTitles(resp)["Arctic Monkeys"] {
		t.Errorf("root dir missing artist child: %v", resp["directory"])
	}
}

func TestGetMusicDirectoryAlbumSongs(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a1.mp3", false)
	_, album, _ := seed(t, ctx, h.q, userAID, n, "Arctic Monkeys", "AM", "Do I Wanna Know?")

	id := encID("al", db.UUIDString(album.ID))
	resp := doGet(h, testAPIKey, "getMusicDirectory", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	if !childTitles(resp)["Do I Wanna Know?"] {
		t.Errorf("album dir missing song child: %v", resp["directory"])
	}
}

func TestGetMusicDirectoryArtistAlbums(t *testing.T) {
	h, ctx, userAID, _ := setupTwo(t)
	n := makeNode(t, ctx, h.q, userAID, pgtype.UUID{}, "a1.mp3", false)
	artist, _, _ := seed(t, ctx, h.q, userAID, n, "Arctic Monkeys", "AM", "Do I Wanna Know?")

	id := encID("ar", db.UUIDString(artist.ID))
	resp := doGet(h, testAPIKey, "getMusicDirectory", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	if !childTitles(resp)["AM"] {
		t.Errorf("artist dir missing album child: %v", resp["directory"])
	}
}

func TestGetMusicDirectoryBadID(t *testing.T) {
	h, _ := setupSubsonic(t)
	resp := doGet(h, testAPIKey, "getMusicDirectory", "id=tr-not-a-real-id")
	if resp["status"] != "failed" {
		t.Errorf("status=%v, want failed", resp["status"])
	}
}
