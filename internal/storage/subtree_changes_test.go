package storage_test

import (
	"context"
	"strings"
	"testing"

	"discodrive/internal/db"
)

// lastSeq returns the max change_log seq for the user (0 if empty).
func lastSeq(t *testing.T, q *db.Queries, userID string) int64 {
	t.Helper()
	ctx := context.Background()
	uid, err := db.ParseUUID(userID)
	if err != nil {
		t.Fatalf("parse uid: %v", err)
	}
	rows, err := q.ListChangesSince(ctx, db.ListChangesSinceParams{UserID: uid, Seq: 0, Lim: 10000})
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	var max int64
	for _, r := range rows {
		if r.Seq > max {
			max = r.Seq
		}
	}
	return max
}

// changedNodesSince returns node_id → disk_path for change_log entries after seq.
func changedNodesSince(t *testing.T, q *db.Queries, userID string, seq int64) map[string]string {
	t.Helper()
	ctx := context.Background()
	uid, err := db.ParseUUID(userID)
	if err != nil {
		t.Fatalf("parse uid: %v", err)
	}
	rows, err := q.ListChangesSince(ctx, db.ListChangesSinceParams{UserID: uid, Seq: seq, Lim: 10000})
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[db.UUIDString(r.NodeID)] = r.DiskPath.String
	}
	return out
}

// A cursor-based scoped feed only ever sees changes recorded AFTER the client's
// cursor. Moving a folder must therefore surface every descendant in the feed,
// not just the folder itself — otherwise a client whose scope covers the target
// location receives an empty directory (795-file Obsidian vault, 2026-07-23).
func TestMoveRecordsSubtreeChanges(t *testing.T) {
	fs, q, userID, _ := setupFS(t)
	ctx := context.Background()

	vault, err := fs.CreateFolder(ctx, userID, nil, "vault")
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID := db.UUIDString(vault.ID)
	note, err := fs.UploadFile(ctx, userID, &vaultID, "note.md", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("upload note: %v", err)
	}
	sub, err := fs.CreateFolder(ctx, userID, &vaultID, "sub")
	if err != nil {
		t.Fatalf("create sub: %v", err)
	}
	subID := db.UUIDString(sub.ID)
	deep, err := fs.UploadFile(ctx, userID, &subID, "deep.md", strings.NewReader("world"))
	if err != nil {
		t.Fatalf("upload deep: %v", err)
	}
	target, err := fs.CreateFolder(ctx, userID, nil, "sync")
	if err != nil {
		t.Fatalf("create sync: %v", err)
	}
	targetID := db.UUIDString(target.ID)

	cursor := lastSeq(t, q, userID)

	if _, err := fs.Move(ctx, userID, vaultID, &targetID); err != nil {
		t.Fatalf("move: %v", err)
	}

	changed := changedNodesSince(t, q, userID, cursor)
	for name, id := range map[string]string{
		"vault":   vaultID,
		"note.md": db.UUIDString(note.ID),
		"sub":     subID,
		"deep.md": db.UUIDString(deep.ID),
	} {
		if _, ok := changed[id]; !ok {
			t.Errorf("no change_log entry after move for %s (node %s)", name, id)
		}
	}
	// Paths reported by the feed must already be the post-move ones.
	if p := changed[db.UUIDString(deep.ID)]; p != "" && p != userID+"/sync/vault/sub/deep.md" {
		t.Errorf("deep.md disk_path in feed = %q, want %q", p, userID+"/sync/vault/sub/deep.md")
	}
}

// Renaming a directory rewrites every descendant's path; clients track paths per
// node, so each descendant must re-enter the change feed with its new path.
func TestRenameRecordsSubtreeChanges(t *testing.T) {
	fs, q, userID, _ := setupFS(t)
	ctx := context.Background()

	dir, err := fs.CreateFolder(ctx, userID, nil, "docs")
	if err != nil {
		t.Fatalf("create docs: %v", err)
	}
	dirID := db.UUIDString(dir.ID)
	file, err := fs.UploadFile(ctx, userID, &dirID, "a.txt", strings.NewReader("a"))
	if err != nil {
		t.Fatalf("upload a.txt: %v", err)
	}

	cursor := lastSeq(t, q, userID)

	if _, err := fs.Rename(ctx, userID, dirID, "papers"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	changed := changedNodesSince(t, q, userID, cursor)
	if _, ok := changed[db.UUIDString(file.ID)]; !ok {
		t.Errorf("no change_log entry after rename for a.txt (node %s)", db.UUIDString(file.ID))
	}
	if p := changed[db.UUIDString(file.ID)]; p != "" && p != userID+"/papers/a.txt" {
		t.Errorf("a.txt disk_path in feed = %q, want %q", p, userID+"/papers/a.txt")
	}
}
