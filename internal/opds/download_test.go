package opds

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// seedDownloadBooks seeds user1 with one real book file on disk, and user2
// with their own book that is NOT shared with user1.
// Returns (user1BookID string, user2BookID string, storageRoot string).
func seedDownloadBooks(t *testing.T, h *Handler) (user1BookID, user2BookID string) {
	t.Helper()
	ctx := t.Context()

	// Write a real file to the handler's storageRoot.
	fileContent := []byte("EPUB-FAKE-CONTENT-0123456789")
	relPath := "books/test-book.epub"
	absPath := filepath.Join(h.storageRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(absPath, fileContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	user1, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail user1: %v", err)
	}

	// Create a file node for user1 pointing to the real file on disk.
	node1, err := h.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   user1.ID,
		ParentID: pgtype.UUID{Valid: false},
		Name:     "test-book.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: relPath, Valid: true},
		Size:     pgtype.Int8{Int64: int64(len(fileContent)), Valid: true},
		Mime:     pgtype.Text{String: "application/epub+zip", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode user1: %v", err)
	}

	book1, err := h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user1.ID,
		NodeID:      node1.ID,
		Title:       "User1 Book",
		SortTitle:   "user1 book",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Size:        pgtype.Int8{Int64: int64(len(fileContent)), Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook user1: %v", err)
	}

	// Create user2 with their own book, NOT shared with user1.
	tenant2, err := h.q.CreateTenant(ctx, "dl-tenant2")
	if err != nil {
		t.Fatalf("CreateTenant user2: %v", err)
	}
	user2, err := h.q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant2.ID,
		Email:        "dl-user2@x.test",
		PasswordHash: "irrelevant",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser user2: %v", err)
	}

	node2, err := h.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   user2.ID,
		ParentID: pgtype.UUID{Valid: false},
		Name:     "secret-book.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "books/secret-book.epub", Valid: true},
		Size:     pgtype.Int8{Int64: 999, Valid: true},
		Mime:     pgtype.Text{String: "application/epub+zip", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode user2: %v", err)
	}

	book2, err := h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user2.ID,
		NodeID:      node2.ID,
		Title:       "User2 Secret Book",
		SortTitle:   "user2 secret book",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Size:        pgtype.Int8{Int64: 999, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook user2: %v", err)
	}

	return db.UUIDString(book1.ID), db.UUIDString(book2.ID)
}

// setupOPDSWithStorage creates a Handler whose storageRoot is a real temp dir.
func setupOPDSWithStorage(t *testing.T) *Handler {
	t.Helper()
	h, _ := setupOPDS(t)
	// Replace storageRoot with a real temp directory.
	root := t.TempDir()
	h.storageRoot = root
	return h
}

// TestDownloadOwnBook verifies that user1 can download their own book (200)
// and that the response body exactly matches the file content.
func TestDownloadOwnBook(t *testing.T) {
	h := setupOPDSWithStorage(t)
	user1BookID, _ := seedDownloadBooks(t, h)

	rec := opdsGetBasic(h, "/opds/download/"+user1BookID, testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	want := []byte("EPUB-FAKE-CONTENT-0123456789")
	if got := rec.Body.Bytes(); string(got) != string(want) {
		t.Errorf("body mismatch: got %q, want %q", got, want)
	}
}

// TestDownloadRangeRequest verifies that a Range header yields 206 Partial Content
// and the correct partial body.
func TestDownloadRangeRequest(t *testing.T) {
	h := setupOPDSWithStorage(t)
	user1BookID, _ := seedDownloadBooks(t, h)

	req := newBasicRequest(http.MethodGet, "/opds/download/"+user1BookID, testEmail, testPassword)
	req.Header.Set("Range", "bytes=0-3")

	rec := doRequest(h, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d — body: %s", rec.Code, rec.Body.String())
	}

	got := rec.Body.String()
	if got != "EPUB" {
		t.Errorf("partial body = %q, want %q", got, "EPUB")
	}

	contentRange := rec.Header().Get("Content-Range")
	if contentRange == "" {
		t.Error("missing Content-Range header on 206 response")
	}
}

// TestDownloadOtherUserBook verifies that user1 cannot download user2's
// unshared book — must get 404, not 200 or 403 (no info leak).
func TestDownloadOtherUserBook(t *testing.T) {
	h := setupOPDSWithStorage(t)
	_, user2BookID := seedDownloadBooks(t, h)

	rec := opdsGetBasic(h, "/opds/download/"+user2BookID, testEmail, testPassword)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for inaccessible book, got %d", rec.Code)
	}
}

// TestDownloadRandomUUID verifies that a valid-format UUID for a non-existent
// book returns 404.
func TestDownloadRandomUUID(t *testing.T) {
	h := setupOPDSWithStorage(t)

	rec := opdsGetBasic(h, "/opds/download/00000000-0000-0000-0000-000000000000", testEmail, testPassword)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown uuid, got %d", rec.Code)
	}
}

// TestDownloadInvalidBookID verifies that a non-UUID path value returns 404.
func TestDownloadInvalidBookID(t *testing.T) {
	h := setupOPDSWithStorage(t)

	rec := opdsGetBasic(h, "/opds/download/not-a-uuid", testEmail, testPassword)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-uuid, got %d", rec.Code)
	}
}

// newBasicRequest builds a request with Basic auth set.
func newBasicRequest(method, target, email, password string) *http.Request {
	req, _ := http.NewRequest(method, target, nil)
	req.SetBasicAuth(email, password)
	return req
}

// doRequest passes a request through the handler and returns the recorder.
func doRequest(h *Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
