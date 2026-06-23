package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
	"discodrive/internal/music"
	"discodrive/internal/storage"
)

// sampleMP3Path is the path to the test fixture mp3 relative to the internal/api
// package directory (which is the working directory during `go test`).
const sampleMP3Path = "../../internal/music/tagwrite/testdata/sample.mp3"

// buildTagEditorEnv sets up a user, a music folder + file node with a real mp3 on
// disk, and a TagEditor. Returns the auth token, the song node ID, the Server, and
// the auth service middleware so callers can wrap handlers.
func buildTagEditorEnv(t *testing.T, email string) (tok, nid string, s *Server) {
	t.Helper()
	ctx := context.Background()

	pool, q, svc := bootstrapPairingDB(t)
	root := t.TempDir()
	fileSvc := storage.NewFileService(pool, storage.NewLocalDisk(root))

	tok2, user, err := svc.Register(ctx, email, "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := user.ID

	// Music folder on disk + DB.
	musicRelDir := db.UUIDString(uid) + "/music"
	if err := os.MkdirAll(filepath.Join(root, musicRelDir), 0o755); err != nil {
		t.Fatalf("mkdir music: %v", err)
	}
	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "music",
		IsDir:    true,
		DiskPath: pgtype.Text{String: musicRelDir, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode folder: %v", err)
	}

	// Music settings: versioning off → Write uses ReplaceContentInPlace (simpler).
	if _, err := q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:            uid,
		Enabled:           true,
		FolderNodeID:      folderNode.ID,
		TagEditVersioning: false,
	}); err != nil {
		t.Fatalf("UpsertMusicSettings: %v", err)
	}

	// Copy the real mp3 fixture into the storage root.
	mp3Bytes, err := os.ReadFile(sampleMP3Path)
	if err != nil {
		t.Fatalf("read sample.mp3: %v", err)
	}
	mp3RelPath := musicRelDir + "/track.mp3"
	if err := os.WriteFile(filepath.Join(root, mp3RelPath), mp3Bytes, 0o644); err != nil {
		t.Fatalf("write track.mp3: %v", err)
	}

	// File node in DB pointing at the mp3 on disk.
	songNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: folderNode.ID,
		Name:     "track.mp3",
		IsDir:    false,
		DiskPath: pgtype.Text{String: mp3RelPath, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode song: %v", err)
	}

	ed := music.NewTagEditor(q, fileSvc, root)
	srv := &Server{auth: svc, q: q, storageRoot: root, tagEditor: ed}
	return tok2, db.UUIDString(songNode.ID), srv
}

// TestPutMusicTags_UpdatesTitle: PUT tags for an mp3 node returns 204 and GET
// reflects the new title — end-to-end TDD anchor for the tag editor API.
func TestPutMusicTags_UpdatesTitle(t *testing.T) {
	tok, nid, s := buildTagEditorEnv(t, "tageditor1@x.test")

	putH := s.auth.Middleware(http.HandlerFunc(s.handlePutMusicTags))
	getH := s.auth.Middleware(http.HandlerFunc(s.handleGetMusicTags))

	// PUT {"fields":["title"],"values":{"title":"Hello"}}
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodPut, "/me/music/tags/"+nid, tok, `{"fields":["title"],"values":{"title":"Hello"}}`, "")
	req.SetPathValue("id", nid)
	putH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	// GET and verify the title was written.
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/music/tags/"+nid, tok, "", "")
	req.SetPathValue("id", nid)
	getH.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"title":"Hello"`) {
		t.Errorf("GET body = %s, want title Hello", rec.Body.String())
	}
}

// TestGetMusicTags_NotAudio: requesting tags for a non-audio file returns 404.
func TestGetMusicTags_NotAudio(t *testing.T) {
	ctx := context.Background()
	pool, q, svc := bootstrapPairingDB(t)
	root := t.TempDir()

	tok, user, err := svc.Register(ctx, "tageditor2@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := user.ID

	// A text file node — no music folder or music settings needed.
	txtRelPath := db.UUIDString(uid) + "/readme.txt"
	_ = os.MkdirAll(filepath.Join(root, db.UUIDString(uid)), 0o755)
	_ = os.WriteFile(filepath.Join(root, txtRelPath), []byte("hi"), 0o644)
	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "readme.txt",
		IsDir:    false,
		DiskPath: pgtype.Text{String: txtRelPath, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	nid := db.UUIDString(fileNode.ID)

	fileSvc := storage.NewFileService(pool, storage.NewLocalDisk(root))
	ed := music.NewTagEditor(q, fileSvc, root)
	s := &Server{auth: svc, q: q, storageRoot: root, tagEditor: ed}

	getH := svc.Middleware(http.HandlerFunc(s.handleGetMusicTags))
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodGet, "/me/music/tags/"+nid, tok, "", "")
	req.SetPathValue("id", nid)
	getH.ServeHTTP(rec, req)
	// ErrNotInMusicFolder or ErrNotAudio → 404.
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET non-audio status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}
