package webdav_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"golang.org/x/net/webdav"

	kfdav "discodrive/internal/webdav"
)

// handlerFor wires the real x/net/webdav Handler on top of our FileSystem, so these
// tests exercise the same PUT path Finder hits (handlePut → OpenFile → Write → Close).
func handlerFor(fs kfdav.FileSystem) *webdav.Handler {
	return &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}
}

// randomBytes builds a deterministic pseudo-random payload: incompressible and, unlike
// a zero buffer, it makes "we stored a hole instead of the data" impossible to miss.
func randomBytes(n int, seed int64) []byte {
	b := make([]byte, n)
	r := rand.New(rand.NewSource(seed))
	_, _ = r.Read(b)
	return b
}

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// TestPutLargeBodyIsStoredByteExact is the control: a complete PUT of a multi-MiB body
// must land byte-for-byte. This is the server-side half of the 2026-07-30 corruption
// report (files stored with the correct length but zeros past offset 262144).
func TestPutLargeBodyIsStoredByteExact(t *testing.T) {
	ctx := context.Background()
	wfs, _, _ := setup(t)
	h := handlerFor(wfs)

	const size = 8 << 20 // 8 MiB: spans many 32 KiB io.Copy iterations and the 256 KiB mark
	payload := randomBytes(size, 1)

	req := httptest.NewRequest(http.MethodPut, "/dav/big.bin", newBodyReader(payload, size))
	req.ContentLength = size
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(kfdav.WithDeclaredLength(ctx, req.ContentLength)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT status = %d, want 201", rec.Code)
	}

	got := readBack(t, wfs, "/big.bin")
	if len(got) != size {
		t.Fatalf("stored size = %d, want %d", len(got), size)
	}
	if sha256hex(got) != sha256hex(payload) {
		t.Fatalf("stored content differs from the uploaded body")
	}
}

// TestPutTruncatedBodyMustNotCommit pins the invariant that a PUT which dies mid-body
// leaves NO file behind. x/net/webdav's handlePut calls f.Close() even when io.Copy
// failed, and writeFile.Close() is what commits to storage — so a client disconnect or
// a proxy timeout silently publishes a truncated file that looks complete in listings
// and whose stored content_hash matches the truncated bytes (making it undetectable).
func TestPutTruncatedBodyMustNotCommit(t *testing.T) {
	ctx := context.Background()
	wfs, _, _ := setup(t)
	h := handlerFor(wfs)

	const declared = 4 << 20  // client promised 4 MiB
	const delivered = 1 << 20 // connection died after 1 MiB
	payload := randomBytes(declared, 2)

	body := &truncatingReader{data: payload, cut: delivered, err: io.ErrUnexpectedEOF}
	req := httptest.NewRequest(http.MethodPut, "/dav/truncated.bin", body)
	req.ContentLength = declared
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(kfdav.WithDeclaredLength(ctx, req.ContentLength)))

	if rec.Code == http.StatusCreated {
		t.Fatalf("PUT reported 201 Created for a body that never finished")
	}

	f, err := wfs.OpenFile(ctx, "/truncated.bin", os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return // correct: the failed upload left nothing behind
		}
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	got, _ := io.ReadAll(f)
	t.Fatalf("failed upload was committed anyway: %d bytes stored (client sent %d of a declared %d)",
		len(got), delivered, declared)
}

// TestPutShortBodyMustNotCommit covers the quieter variant: the body ends cleanly (no
// read error) but delivers fewer bytes than Content-Length promised. Nothing in the
// write path compares the two, so the short file is committed as authoritative.
func TestPutShortBodyMustNotCommit(t *testing.T) {
	ctx := context.Background()
	wfs, _, _ := setup(t)
	h := handlerFor(wfs)

	const declared = 4 << 20
	const delivered = 1 << 20
	payload := randomBytes(delivered, 3)

	req := httptest.NewRequest(http.MethodPut, "/dav/short.bin", newBodyReader(payload, delivered))
	req.ContentLength = declared // promises more than the body carries
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(kfdav.WithDeclaredLength(ctx, req.ContentLength)))

	f, err := wfs.OpenFile(ctx, "/short.bin", os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	got, _ := io.ReadAll(f)
	if len(got) != declared {
		t.Fatalf("stored %d bytes for a PUT that declared Content-Length %d (status %d): "+
			"the short upload was accepted as complete", len(got), declared, rec.Code)
	}
}

func readBack(t *testing.T, wfs kfdav.FileSystem, name string) []byte {
	t.Helper()
	f, err := wfs.OpenFile(context.Background(), name, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func newBodyReader(b []byte, n int) io.Reader {
	return &truncatingReader{data: b[:n], cut: n}
}

// truncatingReader yields data[:cut] and then returns err (nil err = clean EOF).
type truncatingReader struct {
	data []byte
	cut  int
	err  error
	off  int
}

func (r *truncatingReader) Read(p []byte) (int, error) {
	if r.off >= r.cut {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:r.cut])
	r.off += n
	return n, nil
}
