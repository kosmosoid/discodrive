package storage_test

import (
	"context"
	"strings"
	"testing"

	"discodrive/internal/storage"
)

func TestNodeByPathAndRoot(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	if _, err := fs.Push(ctx, userID, nil, "note.txt", nil, "init", strings.NewReader("hi")); err != nil {
		t.Fatalf("push: %v", err)
	}
	dir, err := fs.CreateFolder(ctx, userID, nil, "docs")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	n, err := fs.NodeByPath(ctx, userID, "/note.txt")
	if err != nil || n.Name != "note.txt" {
		t.Fatalf("NodeByPath(/note.txt): n=%v err=%v", n, err)
	}
	if _, err := fs.NodeByPath(ctx, userID, "/missing"); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	roots, err := fs.RootChildren(ctx, userID)
	if err != nil {
		t.Fatalf("RootChildren: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 nodes at root, got %d", len(roots))
	}
	_ = dir
}
