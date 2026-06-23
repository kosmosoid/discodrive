package subsonic

import (
	"os"
	"path/filepath"
	"testing"

	"discodrive/internal/db"
)

func writeSidecarLRC(t *testing.T, storageRoot, relPath, content string) {
	t.Helper()
	abs := filepath.Join(storageRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetLyricsBySongIdSynced(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	userUUID := mustUserID(t, ctx, h.q, testEmail)
	song := seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)
	writeSidecarLRC(t, storageRoot, "music/test-track.lrc", "[00:01.00]Hi\n[00:02.00]There\n")

	id := encID("tr", db.UUIDString(song.ID))
	resp := doGet(h, testAPIKey, "getLyricsBySongId", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	ll, _ := resp["lyricsList"].(map[string]any)
	if ll == nil {
		t.Fatalf("no lyricsList: %v", resp)
	}
	sl, _ := ll["structuredLyrics"].([]any)
	if len(sl) == 0 {
		t.Fatalf("structuredLyrics empty: %v", ll)
	}
	first, _ := sl[0].(map[string]any)
	if first["synced"] != true {
		t.Errorf("synced=%v, want true", first["synced"])
	}
}

func TestGetLyricsBySongIdNone(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	userUUID := mustUserID(t, ctx, h.q, testEmail)
	song := seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)
	id := encID("tr", db.UUIDString(song.ID))
	resp := doGet(h, testAPIKey, "getLyricsBySongId", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok (empty lyrics is not an error)", resp["status"])
	}
	if _, ok := resp["lyricsList"].(map[string]any); !ok {
		t.Fatalf("no lyricsList wrapper: %v", resp)
	}
}

// TestGetLyricsBySongIdLineShape checks that a synced sidecar produces line
// entries with both a non-empty value and a start field.
func TestGetLyricsBySongIdLineShape(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	userUUID := mustUserID(t, ctx, h.q, testEmail)
	song := seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)
	writeSidecarLRC(t, storageRoot, "music/test-track.lrc", "[00:01.00]Hello world\n[00:02.50]Second line\n")

	id := encID("tr", db.UUIDString(song.ID))
	resp := doGet(h, testAPIKey, "getLyricsBySongId", "id="+id)
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	ll, _ := resp["lyricsList"].(map[string]any)
	if ll == nil {
		t.Fatalf("no lyricsList: %v", resp)
	}
	sl, _ := ll["structuredLyrics"].([]any)
	if len(sl) == 0 {
		t.Fatalf("structuredLyrics empty: %v", ll)
	}
	first, _ := sl[0].(map[string]any)
	lines, _ := first["line"].([]any)
	if len(lines) == 0 {
		t.Fatalf("line array empty: %v", first)
	}
	line0, _ := lines[0].(map[string]any)
	if line0 == nil {
		t.Fatalf("line[0] not a map: %v", lines[0])
	}
	val, _ := line0["value"].(string)
	if val == "" {
		t.Errorf("line[0].value is empty, want non-empty")
	}
	if _, ok := line0["start"]; !ok {
		t.Errorf("line[0].start missing for synced lyrics")
	}
}

// TestGetLyricsLegacyHit seeds a song with a sidecar .lrc, calls the legacy
// getLyrics endpoint by title, and asserts a non-empty value and matching title.
func TestGetLyricsLegacyHit(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	userUUID := mustUserID(t, ctx, h.q, testEmail)
	seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)
	// seedSongWithFile seeds the song with title "Test Track" at relpath music/test-track.mp3.
	writeSidecarLRC(t, storageRoot, "music/test-track.lrc", "[00:01.00]Some lyrics here\n[00:02.00]Second line\n")

	resp := doGet(h, testAPIKey, "getLyrics", "title=Test%20Track")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	lyr, _ := resp["lyrics"].(map[string]any)
	if lyr == nil {
		t.Fatalf("no lyrics object: %v", resp)
	}
	if v, _ := lyr["value"].(string); v == "" {
		t.Errorf("lyrics.value is empty, want non-empty")
	}
	if title, _ := lyr["title"].(string); title != "Test Track" {
		t.Errorf("lyrics.title=%q, want %q", title, "Test Track")
	}
}

// TestGetLyricsLegacyMiss calls getLyrics with a title that matches no song.
// The endpoint should return ok with an empty value (not an error).
func TestGetLyricsLegacyMiss(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	// No songs seeded; a title lookup should simply return empty.
	_ = mustUserID(t, ctx, h.q, testEmail)

	resp := doGet(h, testAPIKey, "getLyrics", "title=NoSuchSong123")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok (miss should not be an error)", resp["status"])
	}
	lyr, _ := resp["lyrics"].(map[string]any)
	if lyr == nil {
		t.Fatalf("no lyrics object: %v", resp)
	}
	if v, _ := lyr["value"].(string); v != "" {
		t.Errorf("lyrics.value=%q, want empty string", v)
	}
}
