package storage_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"discodrive/internal/storage"
)

// failingReader yields data[:cut] and then fails, standing in for a chunk upload whose
// connection dies mid-body — the case useUploads.ts explicitly retries (MAX_RETRIES = 3,
// "transient network error: check status and retry the same chunk").
type failingReader struct {
	data []byte
	cut  int
	off  int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.off >= r.cut {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[r.off:r.cut])
	r.off += n
	return n, nil
}

// TestChunkRetryAfterPartialWriteMustNotDuplicateBytes pins the invariant that a chunk
// which failed mid-write leaves nothing behind. Uploads.Chunk appends straight into the
// staging file and does not roll back on error, while nextChunk stays put — so the client's
// retry of the same chunk appends the full chunk AFTER the partial bytes, corrupting the
// file. Nothing downstream catches it: Init never learns the file's total size, so Complete
// pushes whatever accumulated and content_hash is computed over the corrupted bytes.
func TestChunkRetryAfterPartialWriteMustNotDuplicateBytes(t *testing.T) {
	root := t.TempDir()
	st := storage.NewLocalDisk(root)
	u := storage.NewUploads(st, nil)

	chunk := []byte(strings.Repeat("A", 4096))

	id, err := u.Init("u1", nil, "big.bin", int64(len(chunk)))
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// The connection drops after 1000 of the 4096 bytes.
	if _, err := u.Chunk(id, "u1", 0, &failingReader{data: chunk, cut: 1000}); err == nil {
		t.Fatal("Chunk must report the failed body")
	}
	// The client re-sends the same chunk number, exactly as the web UI does.
	if _, err := u.Chunk(id, "u1", 0, bytes.NewReader(chunk)); err != nil {
		t.Fatalf("chunk retry: %v", err)
	}

	f, err := st.Open(".uploads/" + id)
	if err != nil {
		t.Fatalf("open staged file: %v", err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}

	if len(got) != len(chunk) {
		t.Fatalf("staged %d bytes, want %d: the failed chunk's partial bytes were kept and the "+
			"retry appended the whole chunk after them", len(got), len(chunk))
	}
	if !bytes.Equal(got, chunk) {
		t.Fatal("staged content differs from the chunk that was uploaded")
	}
}

// TestCompleteRejectsShortUpload covers the case the size check exists for: the client
// stops sending and calls complete anyway. Without the declared total there is nothing to
// compare against, and the half-uploaded file gets published as finished.
func TestCompleteRejectsShortUpload(t *testing.T) {
	st := storage.NewLocalDisk(t.TempDir())
	u := storage.NewUploads(st, nil) // the size check runs before Push, so no FileService needed

	id, err := u.Init("u1", nil, "big.bin", 8192)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := u.Chunk(id, "u1", 0, strings.NewReader(strings.Repeat("A", 4096))); err != nil {
		t.Fatalf("chunk: %v", err)
	}

	if _, err := u.Complete(t.Context(), id, "u1"); !errors.Is(err, storage.ErrUploadSize) {
		t.Fatalf("Complete on a half-sent upload must be ErrUploadSize, got %v", err)
	}
}

// TestChunkRejectsOvershoot: a chunk that would push the staging file past the declared
// total is refused and rolled back, so the session stays resumable.
func TestChunkRejectsOvershoot(t *testing.T) {
	st := storage.NewLocalDisk(t.TempDir())
	u := storage.NewUploads(st, nil)

	id, err := u.Init("u1", nil, "big.bin", 4096)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := u.Chunk(id, "u1", 0, strings.NewReader(strings.Repeat("A", 4096))); err != nil {
		t.Fatalf("chunk 0: %v", err)
	}
	if _, err := u.Chunk(id, "u1", 1, strings.NewReader(strings.Repeat("B", 4096))); !errors.Is(err, storage.ErrUploadSize) {
		t.Fatalf("overshooting chunk must be ErrUploadSize, got %v", err)
	}

	size, err := st.Size(".uploads/" + id)
	if err != nil {
		t.Fatalf("size: %v", err)
	}
	if size != 4096 {
		t.Fatalf("staging file is %d bytes after the refused chunk, want 4096", size)
	}
}
