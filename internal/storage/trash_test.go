package storage_test

import (
	"context"
	"strings"
	"testing"

	"discodrive/internal/db"
)

func TestPurgeKeepsLiveNodeOnPathCollision(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	// delete "x.txt", create a new live "x.txt" (same path), purge the old one
	oldn, _ := fs.Push(ctx, userID, nil, "x.txt", nil, "i", strings.NewReader("old"))
	oldID := db.UUIDString(oldn.Node.ID)
	if err := fs.Delete(ctx, userID, oldID); err != nil {
		t.Fatalf("del: %v", err)
	}
	newn, _ := fs.Push(ctx, userID, nil, "x.txt", nil, "i", strings.NewReader("new"))
	if err := fs.Purge(ctx, userID, oldID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	// live x.txt must remain (both in DB and readable from disk)
	live, err := fs.NodeByPath(ctx, userID, "/x.txt")
	if err != nil {
		t.Fatalf("live x.txt disappeared from the DB: %v", err)
	}
	_, f, err := fs.Open(ctx, userID, db.UUIDString(live.ID))
	if err != nil {
		t.Fatalf("live x.txt is not readable from disk: %v", err)
	}
	f.Close()
	_ = newn
}

func TestTrashListAndPurge(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	dir, err := fs.CreateFolder(ctx, userID, nil, "docs")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	did := db.UUIDString(dir.ID)
	if _, err := fs.Push(ctx, userID, &did, "inner.txt", nil, "i", strings.NewReader("x")); err != nil {
		t.Fatalf("push: %v", err)
	}
	if _, err := fs.Push(ctx, userID, nil, "top.txt", nil, "i", strings.NewReader("y")); err != nil {
		t.Fatalf("push2: %v", err)
	}

	if err := fs.Delete(ctx, userID, did); err != nil {
		t.Fatalf("del dir: %v", err)
	}
	top, _ := fs.NodeByPath(ctx, userID, "/top.txt")
	if err := fs.Delete(ctx, userID, db.UUIDString(top.ID)); err != nil {
		t.Fatalf("del top: %v", err)
	}

	trash, err := fs.Trash(ctx, userID)
	if err != nil {
		t.Fatalf("trash: %v", err)
	}
	if len(trash) != 2 {
		t.Fatalf("expected 2 top-level items in trash, got %d", len(trash))
	}

	if err := fs.Purge(ctx, userID, did); err != nil {
		t.Fatalf("purge: %v", err)
	}
	trash, _ = fs.Trash(ctx, userID)
	if len(trash) != 1 {
		t.Fatalf("expected 1 in trash after purge, got %d", len(trash))
	}
}

func TestUndelete(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	// (a) restore in place
	n, _ := fs.Push(ctx, userID, nil, "a.txt", nil, "i", strings.NewReader("1"))
	if err := fs.Delete(ctx, userID, db.UUIDString(n.Node.ID)); err != nil {
		t.Fatalf("del: %v", err)
	}
	if _, err := fs.Undelete(ctx, userID, db.UUIDString(n.Node.ID)); err != nil {
		t.Fatalf("undelete: %v", err)
	}
	if _, err := fs.NodeByPath(ctx, userID, "/a.txt"); err != nil {
		t.Fatalf("in place: a.txt did not come back: %v", err)
	}

	// (b) name collision → suffix
	old, _ := fs.Push(ctx, userID, nil, "b.txt", nil, "i", strings.NewReader("old"))
	fs.Delete(ctx, userID, db.UUIDString(old.Node.ID))
	fs.Push(ctx, userID, nil, "b.txt", nil, "i", strings.NewReader("new"))
	if _, err := fs.Undelete(ctx, userID, db.UUIDString(old.Node.ID)); err != nil {
		t.Fatalf("undelete collision: %v", err)
	}
	if _, err := fs.NodeByPath(ctx, userID, "/b.txt (restored)"); err != nil {
		t.Fatalf("collision: expected a suffix, %v", err)
	}

	// (c) parent is in trash → restore to root
	dir, _ := fs.CreateFolder(ctx, userID, nil, "d")
	did := db.UUIDString(dir.ID)
	child, _ := fs.Push(ctx, userID, &did, "c.txt", nil, "i", strings.NewReader("c"))
	fs.Delete(ctx, userID, did)
	if _, err := fs.Undelete(ctx, userID, db.UUIDString(child.Node.ID)); err != nil {
		t.Fatalf("undelete child: %v", err)
	}
	if _, err := fs.NodeByPath(ctx, userID, "/c.txt"); err != nil {
		t.Fatalf("parent in trash: c.txt not at root: %v", err)
	}
}

func TestUndeleteFolderSubtree(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	dir, _ := fs.CreateFolder(ctx, userID, nil, "proj")
	did := db.UUIDString(dir.ID)
	fs.Push(ctx, userID, &did, "a.txt", nil, "i", strings.NewReader("a"))
	sub, _ := fs.CreateFolder(ctx, userID, &did, "sub")
	sid := db.UUIDString(sub.ID)
	fs.Push(ctx, userID, &sid, "b.txt", nil, "i", strings.NewReader("b"))

	if err := fs.Delete(ctx, userID, did); err != nil {
		t.Fatalf("del: %v", err)
	}
	if _, err := fs.Undelete(ctx, userID, did); err != nil {
		t.Fatalf("undelete folder: %v", err)
	}
	for _, p := range []string{"/proj", "/proj/a.txt", "/proj/sub", "/proj/sub/b.txt"} {
		if _, err := fs.NodeByPath(ctx, userID, p); err != nil {
			t.Fatalf("after restoring the folder, %s is missing: %v", p, err)
		}
	}
}

func TestPurgeAll(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	dir, _ := fs.CreateFolder(ctx, userID, nil, "d")
	fs.Delete(ctx, userID, db.UUIDString(dir.ID))
	f, _ := fs.Push(ctx, userID, nil, "f.txt", nil, "i", strings.NewReader("x"))
	fs.Delete(ctx, userID, db.UUIDString(f.Node.ID))

	if trash, _ := fs.Trash(ctx, userID); len(trash) != 2 {
		t.Fatalf("expected 2 in trash, got %d", len(trash))
	}
	if err := fs.PurgeAll(ctx, userID); err != nil {
		t.Fatalf("PurgeAll: %v", err)
	}
	if trash, _ := fs.Trash(ctx, userID); len(trash) != 0 {
		t.Fatalf("trash is not empty after PurgeAll: %d", len(trash))
	}
}

// Regression: restoring with a name collision must NOT clobber the LIVE node sharing
// the same disk_path (bug: a broad RewriteSubtreePaths was rewriting its disk_path to
// the suffixed version while keeping name intact → name≠disk_path → broke PROPFIND/listing).
func TestUndeleteCollisionDoesNotClobberLiveNode(t *testing.T) {
	ctx := context.Background()
	fs, _, uid, _ := setupFS(t)

	a, _ := fs.Push(ctx, uid, nil, "X.txt", nil, "", strings.NewReader("old"))
	if err := fs.Delete(ctx, uid, db.UUIDString(a.Node.ID)); err != nil {
		t.Fatalf("del: %v", err)
	}
	b, _ := fs.Push(ctx, uid, nil, "X.txt", nil, "", strings.NewReader("new")) // live node, same path
	if _, err := fs.Undelete(ctx, uid, db.UUIDString(a.Node.ID)); err != nil {
		t.Fatalf("undelete: %v", err)
	}

	// live X.txt (b) is intact: /X.txt resolves to b, content is preserved
	liveX, err := fs.NodeByPath(ctx, uid, "/X.txt")
	if err != nil {
		t.Fatalf("live X.txt disappeared/corrupted: %v", err)
	}
	if db.UUIDString(liveX.ID) != db.UUIDString(b.Node.ID) {
		t.Fatalf("/X.txt does not resolve to the live node")
	}
	if got := readNode(t, fs, uid, db.UUIDString(b.Node.ID)); got != "new" {
		t.Fatalf("content of live X.txt=%q, expected new", got)
	}
	// the restored copy is alongside it
	if _, err := fs.NodeByPath(ctx, uid, "/X.txt (restored)"); err != nil {
		t.Fatalf("restored copy is missing: %v", err)
	}
}

// Regression: soft-delete leaves the file on disk; rescan must NOT resurrect it
// as a new live node (otherwise deletion via WebDAV/UI "doesn't stick").
func TestRescanDoesNotResurrectTrashed(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	n, _ := fs.Push(ctx, userID, nil, "x.txt", nil, "i", strings.NewReader("x"))
	if err := fs.Delete(ctx, userID, db.UUIDString(n.Node.ID)); err != nil {
		t.Fatalf("del: %v", err)
	}
	// file is still on disk — rescan must not create a new live node
	if err := fs.Rescan(ctx); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if trash, _ := fs.Trash(ctx, userID); len(trash) != 1 {
		t.Fatalf("expected 1 in trash, got %d", len(trash))
	}
	if roots, _ := fs.RootChildren(ctx, userID); len(roots) != 0 {
		t.Fatalf("rescan resurrected the node: %d live at root", len(roots))
	}
}
