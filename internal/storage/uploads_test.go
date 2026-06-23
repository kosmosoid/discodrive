package storage_test

import (
	"errors"
	"strings"
	"testing"

	"discodrive/internal/storage"
)

func TestUploadAbort(t *testing.T) {
	st := storage.NewLocalDisk(t.TempDir())
	u := storage.NewUploads(st, nil) // Init/Chunk/Status/Abort do not use FileService

	id, err := u.Init("u1", nil, "f.txt")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := u.Chunk(id, "u1", 0, strings.NewReader("data")); err != nil {
		t.Fatalf("chunk: %v", err)
	}

	u.Abort("u1", id)

	if _, err := u.Status(id, "u1"); !errors.Is(err, storage.ErrUploadNotFound) {
		t.Fatalf("after abort, Status must be ErrUploadNotFound, got %v", err)
	}
	if _, err := u.Chunk(id, "u1", 1, strings.NewReader("x")); !errors.Is(err, storage.ErrUploadNotFound) {
		t.Fatalf("after abort, Chunk must be ErrUploadNotFound, got %v", err)
	}
	u.Abort("u1", "nope") // unknown id — must not panic
}
