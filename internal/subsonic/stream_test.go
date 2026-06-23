package subsonic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
	"discodrive/internal/secret"
)

// fakeAudio is a ~2 KB payload used as a stand-in audio file in stream tests.
var fakeAudio = bytes.Repeat([]byte("AUDIO_DATA_"), 200) // 2200 bytes

// setupWithPool spins up a Postgres test container and returns a ready Handler,
// context, and the underlying pgxpool (needed for raw SQL assertions in tests).
// The test user has the standard testEmail/testAPIKey credentials.
func setupWithPool(t *testing.T) (*Handler, context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("Docker required: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	dsn, _ := pgC.ConnectionString(ctx, "sslmode=disable")
	if err := db.MigrateUp(dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	q := db.New(pool)

	// Create tenant + user.
	tenant, err := q.CreateTenant(ctx, "test")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        testEmail,
		PasswordHash: "irrelevant",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	cipher, err := secret.New(testKey)
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	ct, err := cipher.Encrypt(testPassword)
	if err != nil {
		t.Fatalf("cipher.Encrypt: %v", err)
	}

	if _, err = q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  user.ID,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertMusicSettings: %v", err)
	}
	if err = q.SetMusicCredentials(ctx, db.SetMusicCredentialsParams{
		UserID:         user.ID,
		PasswordCipher: pgtype.Text{String: ct, Valid: true},
		ApiKey:         pgtype.Text{String: testAPIKey, Valid: true},
	}); err != nil {
		t.Fatalf("SetMusicCredentials: %v", err)
	}

	h := New(q, cipher, nil, "", false)
	return h, ctx, pool
}

// seedSongWithFile writes payload to disk under storageRoot, creates a node row
// that points to that file, and seeds artist/album/song records with ContentType set.
func seedSongWithFile(
	t *testing.T,
	ctx context.Context,
	q *db.Queries,
	userID pgtype.UUID,
	storageRoot string,
	payload []byte,
) db.Song {
	t.Helper()

	relPath := "music/test-track.mp3"
	absPath := filepath.Join(storageRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absPath, payload, 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	// Create a node row pointing to the file on disk.
	node, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   userID,
		Name:     "test-track.mp3",
		IsDir:    false,
		DiskPath: pgtype.Text{String: relPath, Valid: true},
		Mime:     pgtype.Text{String: "audio/mpeg", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode(audio): %v", err)
	}

	// Seed artist/album/song; then update the song with ContentType and Size.
	_, _, baseSong := seed(t, ctx, q, userID, node.ID, "Test Artist", "Test Album", "Test Track")

	song, err := q.UpsertSong(ctx, db.UpsertSongParams{
		UserID:      userID,
		AlbumID:     baseSong.AlbumID,
		ArtistID:    baseSong.ArtistID,
		NodeID:      node.ID,
		Title:       baseSong.Title,
		Duration:    pgtype.Int4{Int32: 180, Valid: true},
		Track:       pgtype.Int4{Int32: 1, Valid: true},
		Suffix:      pgtype.Text{String: "mp3", Valid: true},
		ContentType: pgtype.Text{String: "audio/mpeg", Valid: true},
		Size:        pgtype.Int8{Int64: int64(len(payload)), Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSong(contentType): %v", err)
	}
	return song
}

// TestStreamServesFullFile verifies that GET /rest/stream?id=tr-<uuid> returns 200
// with the full audio payload and an Accept-Ranges header.
func TestStreamServesFullFile(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	h.xaccel = false

	userUUID := mustUserID(t, ctx, h.q, testEmail)
	song := seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)

	songID := encID("tr", db.UUIDString(song.ID))
	target := "/rest/stream?id=" + songID + "&apiKey=" + testAPIKey + "&f=json&c=test&v=1.16.1"

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, fakeAudio) {
		t.Errorf("body mismatch: got %d bytes, want %d bytes", len(got), len(fakeAudio))
	}
	if ar := rec.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("Accept-Ranges = %q, want \"bytes\"", ar)
	}
}

// TestStreamRange verifies that a Range request returns 206 Partial Content
// with a Content-Range header and the requested byte slice.
func TestStreamRange(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	h.xaccel = false

	userUUID := mustUserID(t, ctx, h.q, testEmail)
	song := seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)

	songID := encID("tr", db.UUIDString(song.ID))
	target := "/rest/stream?id=" + songID + "&apiKey=" + testAPIKey + "&f=json&c=test&v=1.16.1"

	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Range", "bytes=0-9")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d — body: %s", rec.Code, rec.Body.String())
	}
	if cr := rec.Header().Get("Content-Range"); cr == "" {
		t.Errorf("Content-Range header missing")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, fakeAudio[:10]) {
		t.Errorf("range body = %q, want first 10 bytes", got)
	}
}

// TestStreamForeignSongDenied verifies that a user cannot stream a song owned by
// another user that has not been shared with them.
func TestStreamForeignSongDenied(t *testing.T) {
	storageRoot := t.TempDir()

	h, ctx, _ := setupWithPool(t)
	h.storageRoot = storageRoot
	h.xaccel = false

	userAUUID := mustUserID(t, ctx, h.q, testEmail)

	// Create user B with their own music settings.
	uA, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	uB, err := h.q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     uA.TenantID,
		Email:        "b-stream@x.test",
		PasswordHash: "irrelevant",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser(B): %v", err)
	}
	if _, err = h.q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  uB.ID,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertMusicSettings(B): %v", err)
	}

	// Seed a song owned by user B (not shared to A).
	songB := seedSongWithFile(t, ctx, h.q, uB.ID, storageRoot, fakeAudio)
	_ = userAUUID // user A is authenticated via testAPIKey

	// User A attempts to stream user B's song.
	songID := encID("tr", db.UUIDString(songB.ID))
	target := "/rest/stream?id=" + songID + "&apiKey=" + testAPIKey + "&f=json&c=test&v=1.16.1"

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v — body: %s", err, rec.Body.String())
	}
	inner := subsonicResponse(m)
	if inner == nil {
		t.Fatalf("no subsonic-response: %s", rec.Body.String())
	}
	if inner["status"] != "failed" {
		t.Errorf("status = %v, want failed", inner["status"])
	}
	errObj, _ := inner["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("no error object: %v", inner)
	}
	if code, _ := errObj["code"].(float64); int(code) != ErrNotFound {
		t.Errorf("error.code = %v, want %d", errObj["code"], ErrNotFound)
	}
}

// TestScrobbleRecordsHistory verifies that a valid scrobble request inserts a
// play_history row for the (user, song) pair.
func TestScrobbleRecordsHistory(t *testing.T) {
	storageRoot := t.TempDir()
	h, ctx, pool := setupWithPool(t)
	h.storageRoot = storageRoot

	userUUID := mustUserID(t, ctx, h.q, testEmail)
	song := seedSongWithFile(t, ctx, h.q, userUUID, storageRoot, fakeAudio)

	songID := encID("tr", db.UUIDString(song.ID))
	target := "/rest/scrobble?id=" + songID + "&apiKey=" + testAPIKey + "&f=json&c=test&v=1.16.1"

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v — body: %s", err, rec.Body.String())
	}
	inner := subsonicResponse(m)
	if inner == nil || inner["status"] != "ok" {
		t.Fatalf("scrobble failed: %s", rec.Body.String())
	}

	// Verify a play_history row was inserted for this user+song.
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM play_history WHERE user_id = $1 AND song_id = $2",
		userUUID, song.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count play_history: %v", err)
	}
	if count != 1 {
		t.Errorf("play_history row count = %d, want 1", count)
	}
}
