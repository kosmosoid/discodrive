package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// 11 file versions → TrimVersions keeps 10 (disk + file_versions), oldest is removed.
func TestTrimVersions_KeepsTen(t *testing.T) {
	ctx := context.Background()
	fs, q, userID, root := setupFS(t)

	var nodeID string
	for i := 1; i <= 11; i++ {
		res, err := fs.Push(ctx, userID, nil, "multi.txt", nil, "", strings.NewReader(fmt.Sprintf("rev-%d", i)))
		if err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
		nodeID = db.UUIDString(res.Node.ID)
	}
	nid, _ := db.ParseUUID(nodeID)

	before, err := q.ListFileVersions(ctx, nid)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(before) != 11 {
		t.Fatalf("versions before trimming=%d, expected 11", len(before))
	}

	if err := fs.TrimVersions(ctx, 10); err != nil {
		t.Fatalf("trim: %v", err)
	}

	after, err := q.ListFileVersions(ctx, nid)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(after) != 10 {
		t.Fatalf("versions after trimming=%d, expected 10", len(after))
	}
	// 10 newest versions remain (2..11), v1 is deleted
	for _, v := range after {
		if v.Version == 1 {
			t.Fatal("version 1 (the oldest) must be deleted")
		}
	}
	// 10 snapshots on disk as well
	snapDir := filepath.Join(root, ".versions", userID, nodeID)
	ents, err := os.ReadDir(snapDir)
	if err != nil {
		t.Fatalf("read snapshot dir: %v", err)
	}
	if len(ents) != 10 {
		t.Fatalf("snapshots on disk=%d, expected 10", len(ents))
	}
}

// A file added to disk outside the API appears in nodes + change_log after Rescan;
// one deleted by hand is soft-deleted.
func TestRescan_DetectsManualAddAndDelete(t *testing.T) {
	ctx := context.Background()
	fs, q, userID, root := setupFS(t)
	uid, _ := db.ParseUUID(userID)

	// drop a file manually into the user mirror
	rel := userID + "/dropped.txt"
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("manual content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := fs.Rescan(ctx); err != nil {
		t.Fatalf("rescan add: %v", err)
	}

	node, err := q.GetLiveNodeByPath(ctx, db.GetLiveNodeByPathParams{UserID: uid, Path: rel})
	if err != nil {
		t.Fatalf("node not found after rescan: %v", err)
	}
	if node.Size.Int64 != int64(len("manual content")) {
		t.Fatalf("size=%d, expected %d", node.Size.Int64, len("manual content"))
	}
	if !hasChange(t, ctx, q, uid, "create") {
		t.Fatal("change_log has no create for the added file")
	}

	// remove the file by hand
	if err := os.Remove(full); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := fs.Rescan(ctx); err != nil {
		t.Fatalf("rescan delete: %v", err)
	}

	_, err = q.GetLiveNodeByPath(ctx, db.GetLiveNodeByPathParams{UserID: uid, Path: rel})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("node must be soft-deleted, but it is alive (err=%v)", err)
	}
	if !hasChange(t, ctx, q, uid, "delete") {
		t.Fatal("change_log has no delete for the removed file")
	}
}

func hasChange(t *testing.T, ctx context.Context, q *db.Queries, uid pgtype.UUID, op string) bool {
	t.Helper()
	rows, err := q.ListChangesSince(ctx, db.ListChangesSinceParams{UserID: uid, Seq: 0, Lim: 1000})
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	for _, r := range rows {
		if r.Op == op {
			return true
		}
	}
	return false
}
