package storage_test

import (
	"context"
	"strings"
	"testing"

	"discodrive/internal/db"
)

func TestReplaceContentInPlace_NoNewVersion(t *testing.T) {
	ctx := context.Background()
	fs, q, userID, _ := setupFS(t)

	base, err := fs.UploadFile(ctx, userID, nil, "song.mp3", strings.NewReader("AAAA"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	versionsBefore, _ := q.ListFileVersions(ctx, base.ID)

	updated, err := fs.ReplaceContentInPlace(ctx, userID, db.UUIDString(base.ID), strings.NewReader("BBBBBB"))
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if updated.Size.Int64 != 6 {
		t.Errorf("size = %d, want 6", updated.Size.Int64)
	}
	versionsAfter, _ := q.ListFileVersions(ctx, base.ID)
	if len(versionsAfter) != len(versionsBefore) {
		t.Errorf("versions changed: before=%d after=%d, want equal", len(versionsBefore), len(versionsAfter))
	}
}
