package storage_test

import (
	"context"
	"strings"
	"testing"

	"discodrive/internal/db"
	"discodrive/internal/storage"
)

// TestPushByPath verifies sync-push by user-relative path (stage 3.2b).
func TestPushByPath(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	// 1. First push to a nested path — directories a/b are created automatically.
	res1, err := fs.PushByPath(ctx, userID, "a/b/c.txt", nil, strings.NewReader("hi"))
	if err != nil {
		t.Fatalf("PushByPath first: %v", err)
	}
	if res1.Conflicted {
		t.Fatal("first push must not be a conflict")
	}

	// Verify disk_path = <userID>/a/b/c.txt.
	wantDiskPath := userID + "/a/b/c.txt"
	if res1.Node.DiskPath.String != wantDiskPath {
		t.Fatalf("disk_path = %q, expected %q", res1.Node.DiskPath.String, wantDiskPath)
	}

	// Read content via fs.Open.
	got := readNode(t, fs, userID, db.UUIDString(res1.Node.ID))
	if got != "hi" {
		t.Fatalf("content = %q, expected %q", got, "hi")
	}

	// 2. Re-push with a correct baseVersion → Conflicted=false.
	res2, err := fs.PushByPath(ctx, userID, "a/b/c.txt", &res1.Node.Version, strings.NewReader("hi2"))
	if err != nil {
		t.Fatalf("PushByPath repeated: %v", err)
	}
	if res2.Conflicted {
		t.Fatalf("a correct base_version must not cause a conflict")
	}

	// 3. Push with a stale baseVersion → Conflicted=true.
	wrongVersion := int64(9999)
	res3, err := fs.PushByPath(ctx, userID, "a/b/c.txt", &wrongVersion, strings.NewReader("conflict"))
	if err != nil {
		t.Fatalf("PushByPath conflicting: %v", err)
	}
	if !res3.Conflicted {
		t.Fatal("a stale base_version must cause a conflict")
	}

	// 4. DeleteByPath — idempotent.
	if err := fs.DeleteByPath(ctx, userID, "a/b/c.txt"); err != nil {
		t.Fatalf("DeleteByPath first: %v", err)
	}
	if err := fs.DeleteByPath(ctx, userID, "a/b/c.txt"); err != nil {
		t.Fatalf("DeleteByPath repeated (idempotency): %v", err)
	}

	// 5. EnsureDirByPath — idempotent directory creation.
	dir1, err := fs.EnsureDirByPath(ctx, userID, "x/y/z")
	if err != nil {
		t.Fatalf("EnsureDirByPath: %v", err)
	}
	if !dir1.IsDir {
		t.Fatal("EnsureDirByPath must return a folder")
	}
	// Second call → same node.
	dir2, err := fs.EnsureDirByPath(ctx, userID, "x/y/z")
	if err != nil {
		t.Fatalf("EnsureDirByPath repeated: %v", err)
	}
	if dir1.ID != dir2.ID {
		t.Fatal("EnsureDirByPath must return the same node when repeated")
	}

	// 6. PushByPath to root (no subdirectories).
	res4, err := fs.PushByPath(ctx, userID, "root.txt", nil, strings.NewReader("root"))
	if err != nil {
		t.Fatalf("PushByPath to root: %v", err)
	}
	if res4.Node.Name != "root.txt" {
		t.Fatalf("node name = %q, expected root.txt", res4.Node.Name)
	}

	// 7. PushByPath with an empty path → ErrNotFound.
	_, err = fs.PushByPath(ctx, userID, "", nil, strings.NewReader(""))
	if err != storage.ErrNotFound {
		t.Fatalf("an empty path must return ErrNotFound, got %v", err)
	}
}
