package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"discodrive/internal/storage"
)

// TestPushKeepsClientModifiedAt: a file whose content is from 2019 must read as 2019 after
// being uploaded today. This is what makes mobile auto-upload of a camera roll usable.
func TestPushKeepsClientModifiedAt(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	want := time.Date(2019, 6, 15, 12, 30, 0, 0, time.UTC)
	res, err := fs.PushWithMeta(ctx, userID, nil, "photo.jpg", nil, "phone",
		strings.NewReader("jpegbytes"), storage.PushMeta{ModifiedAt: want})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !res.Node.ModifiedAt.Valid {
		t.Fatal("modified_at is null")
	}
	if got := res.Node.ModifiedAt.Time.UTC(); !got.Equal(want) {
		t.Fatalf("modified_at = %s, want %s", got, want)
	}
}

// TestPushWithoutMetaUsesServerTime pins the compatibility promise: a client that sends
// nothing keeps getting the upload time, exactly as before.
func TestPushWithoutMetaUsesServerTime(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	before := time.Now().Add(-time.Minute)
	res, err := fs.Push(ctx, userID, nil, "note.txt", nil, "web", strings.NewReader("hi"))
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !res.Node.ModifiedAt.Valid {
		t.Fatal("modified_at is null")
	}
	if got := res.Node.ModifiedAt.Time; got.Before(before) {
		t.Fatalf("modified_at = %s, want roughly now — the server date was not applied", got)
	}
}

// TestPushUpdateKeepsClientModifiedAt: overwriting a file adopts the new content's date
// rather than stamping the moment of upload.
func TestPushUpdateKeepsClientModifiedAt(t *testing.T) {
	ctx := context.Background()
	fs, _, userID, _ := setupFS(t)

	if _, err := fs.Push(ctx, userID, nil, "doc.txt", nil, "web", strings.NewReader("v1")); err != nil {
		t.Fatalf("first push: %v", err)
	}
	want := time.Date(2021, 3, 2, 8, 0, 0, 0, time.UTC)
	res, err := fs.PushWithMeta(ctx, userID, nil, "doc.txt", nil, "web",
		strings.NewReader("v2"), storage.PushMeta{ModifiedAt: want})
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if res.Node.Version != 2 {
		t.Fatalf("version = %d, want 2", res.Node.Version)
	}
	if got := res.Node.ModifiedAt.Time.UTC(); !got.Equal(want) {
		t.Fatalf("modified_at after update = %s, want %s", got, want)
	}
}
