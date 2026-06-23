package storage_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"discodrive/internal/db"
	"discodrive/internal/storage"
)

func newUser(t *testing.T, ctx context.Context, q *db.Queries, email string) string {
	t.Helper()
	tenant, err := q.CreateTenant(ctx, email)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID: tenant.ID, Email: email, PasswordHash: "x", Role: "user",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return db.UUIDString(u.ID)
}

// Sharing a folder with read to user B → B can read a nested file (downward inheritance)
// but cannot write.
func TestShare_FolderReadInheritedToChild(t *testing.T) {
	ctx := context.Background()
	fs, q, alice, _ := setupFS(t)
	bob := newUser(t, ctx, q, "bob@x")

	folder, err := fs.CreateFolder(ctx, alice, nil, "Docs")
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	folderID := db.UUIDString(folder.ID)
	file, err := fs.Push(ctx, alice, &folderID, "secret.txt", nil, "", strings.NewReader("hi"))
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	fileID := db.UUIDString(file.Node.ID)

	// before sharing, B cannot see the nested file
	if _, err := fs.NodeForDownload(ctx, bob, fileID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("before sharing, B must not have access, err=%v", err)
	}

	// A shares the FOLDER (read) → access is inherited by the nested file
	if _, err := fs.ShareToUser(ctx, alice, folderID, "bob@x", "read", nil); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := fs.NodeForDownload(ctx, bob, fileID); err != nil {
		t.Fatalf("after sharing the folder, B must read the nested file (inheritance), err=%v", err)
	}
	// read does not grant write (recipient is not the owner)
	if _, err := fs.Rename(ctx, bob, fileID, "x.txt"); !errors.Is(err, storage.ErrNotOwner) {
		t.Fatalf("recipient must not rename, err=%v", err)
	}
}

// The recipient is read-only for now: even read_write does not allow mutations (write-sharing deferred).
func TestShare_RecipientIsReadOnly(t *testing.T) {
	ctx := context.Background()
	fs, q, alice, _ := setupFS(t)
	bob := newUser(t, ctx, q, "bob@x")

	folder, _ := fs.CreateFolder(ctx, alice, nil, "shared")
	folderID := db.UUIDString(folder.ID)
	file, _ := fs.Push(ctx, alice, &folderID, "doc.txt", nil, "", strings.NewReader("v"))
	fileID := db.UUIDString(file.Node.ID)

	if _, err := fs.ShareToUser(ctx, alice, folderID, "bob@x", "read_write", nil); err != nil {
		t.Fatalf("share rw: %v", err)
	}

	// read/download/list — allowed
	if _, err := fs.NodeForDownload(ctx, bob, fileID); err != nil {
		t.Fatalf("recipient must be able to read, err=%v", err)
	}
	if _, err := fs.ListChildren(ctx, bob, folderID); err != nil {
		t.Fatalf("recipient must be able to list, err=%v", err)
	}
	// mutations — denied (ErrNotOwner), even with read_write
	if _, err := fs.Rename(ctx, bob, fileID, "no.txt"); !errors.Is(err, storage.ErrNotOwner) {
		t.Fatalf("rename must be denied, err=%v", err)
	}
	if err := fs.Delete(ctx, bob, fileID); !errors.Is(err, storage.ErrNotOwner) {
		t.Fatalf("delete must be denied, err=%v", err)
	}
	if _, err := fs.Move(ctx, bob, fileID, nil); !errors.Is(err, storage.ErrNotOwner) {
		t.Fatalf("move must be denied, err=%v", err)
	}
	if _, err := fs.Push(ctx, bob, &folderID, "new.txt", nil, "", strings.NewReader("x")); !errors.Is(err, storage.ErrNotOwner) {
		t.Fatalf("upload into someone else's folder must be denied, err=%v", err)
	}
}

// An expired link is closed; an active one is open.
func TestShare_LinkExpiry(t *testing.T) {
	ctx := context.Background()
	fs, _, alice, _ := setupFS(t)

	file, err := fs.Push(ctx, alice, nil, "pub.txt", nil, "", strings.NewReader("p"))
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	fileID := db.UUIDString(file.Node.ID)

	past := time.Now().Add(-time.Hour)
	_, expiredTok, err := fs.ShareByLink(ctx, alice, fileID, "read", &past)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := fs.NodeByLink(ctx, expiredTok); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("an expired link must be closed, err=%v", err)
	}

	_, liveTok, err := fs.ShareByLink(ctx, alice, fileID, "read", nil)
	if err != nil {
		t.Fatalf("link2: %v", err)
	}
	if _, err := fs.NodeByLink(ctx, liveTok); err != nil {
		t.Fatalf("an active link must work, err=%v", err)
	}
}

// Revoking a share closes access immediately.
func TestShare_RevokeClosesAccess(t *testing.T) {
	ctx := context.Background()
	fs, q, alice, _ := setupFS(t)
	bob := newUser(t, ctx, q, "bob@x")

	file, err := fs.Push(ctx, alice, nil, "r.txt", nil, "", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	fileID := db.UUIDString(file.Node.ID)

	share, err := fs.ShareToUser(ctx, alice, fileID, "bob@x", "read", nil)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := fs.NodeForDownload(ctx, bob, fileID); err != nil {
		t.Fatalf("before revocation B must be able to read, err=%v", err)
	}
	if err := fs.Revoke(ctx, alice, db.UUIDString(share.ID)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := fs.NodeForDownload(ctx, bob, fileID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("after revocation access must be closed, err=%v", err)
	}
}

func TestSharesForNode(t *testing.T) {
	ctx := context.Background()
	fs, q, userID, _ := setupFS(t)
	tenant, _ := q.CreateTenant(ctx, "t2")
	other, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "b@x", PasswordHash: "x", Role: "user"})
	_ = other

	n, _ := fs.Push(ctx, userID, nil, "f.txt", nil, "i", strings.NewReader("z"))
	id := db.UUIDString(n.Node.ID)
	if _, err := fs.ShareToUser(ctx, userID, id, "b@x", "read", nil); err != nil {
		t.Fatalf("share: %v", err)
	}
	shares, err := fs.SharesForNode(ctx, userID, id)
	if err != nil {
		t.Fatalf("SharesForNode: %v", err)
	}
	if len(shares) != 1 || shares[0].Access != "read" {
		t.Fatalf("expected 1 read share, got %+v", shares)
	}
}

func TestLeaveShare(t *testing.T) {
	ctx := context.Background()
	fs, q, alice, _ := setupFS(t)
	bob := newUser(t, ctx, q, "bob@x")

	file, _ := fs.Push(ctx, alice, nil, "f.txt", nil, "", strings.NewReader("z"))
	fileID := db.UUIDString(file.Node.ID)
	share, err := fs.ShareToUser(ctx, alice, fileID, "bob@x", "read", nil)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	sid := db.UUIDString(share.ID)

	// a non-recipient cannot "leave" someone else's share
	if err := fs.LeaveShare(ctx, alice, sid); !errors.Is(err, storage.ErrNotOwner) {
		t.Fatalf("alice is not a grantee — expected ErrNotOwner, got %v", err)
	}
	// the recipient removes their share
	if err := fs.LeaveShare(ctx, bob, sid); err != nil {
		t.Fatalf("leave: %v", err)
	}
	// access is gone
	if _, err := fs.NodeForDownload(ctx, bob, fileID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("after leave there must be no access, err=%v", err)
	}
}
