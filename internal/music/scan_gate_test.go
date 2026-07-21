package music

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// A file whose duration cannot be probed (garbage bytes with an audio
// extension) must be indexed once and then left alone: the change-gate must
// not re-index it on every scan just because duration stayed unknown.
func TestScanFolder_UnprobeableDurationIndexedOnce(t *testing.T) {
	q, ctx := setupDB(t)
	userID := makeTenant(t, q, ctx)
	uid, _ := db.ParseUUID(userID)

	storageRoot := t.TempDir()
	ix := NewIndexer(q, storageRoot)

	musicDir := filepath.Join(storageRoot, "music")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatalf("mkdir music: %v", err)
	}
	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID: uid, Name: "music", IsDir: true,
		DiskPath: pgtype.Text{String: "music", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (folder): %v", err)
	}

	// Garbage bytes: ReadMeta tolerates them (falls back to the filename), but
	// no duration can ever be extracted.
	if err := os.WriteFile(filepath.Join(musicDir, "noise.ogg"), []byte("not really ogg"), 0o644); err != nil {
		t.Fatalf("write noise.ogg: %v", err)
	}
	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID: uid, ParentID: folderNode.ID, Name: "noise.ogg", IsDir: false,
		DiskPath: pgtype.Text{String: "music/noise.ogg", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (noise): %v", err)
	}

	folderID := db.UUIDString(folderNode.ID)
	count, err := ix.ScanFolder(ctx, userID, folderID)
	if err != nil {
		t.Fatalf("ScanFolder: %v", err)
	}
	if count != 1 {
		t.Fatalf("first ScanFolder: count=%d, want 1", count)
	}
	if _, err := q.GetSongByNode(ctx, fileNode.ID); err != nil {
		t.Fatalf("song not indexed: %v", err)
	}

	count2, err := ix.ScanFolder(ctx, userID, folderID)
	if err != nil {
		t.Fatalf("ScanFolder (second): %v", err)
	}
	if count2 != 0 {
		t.Errorf("second ScanFolder: count=%d, want 0 — unprobeable file re-indexed forever", count2)
	}
}
