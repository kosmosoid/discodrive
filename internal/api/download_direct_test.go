package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestStreamFileServesBytesAndRange(t *testing.T) {
	root := t.TempDir()
	content := []byte("привет, это содержимое файла для стрима")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{storageRoot: root}
	mime := pgtype.Text{String: "text/plain", Valid: true}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files/x/content", nil)
	s.streamFile(rec, req, mime, "f.txt", "f.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	if got, _ := io.ReadAll(rec.Body); string(got) != string(content) {
		t.Fatalf("body did not match: %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("Content-Type %q", ct)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/files/x/content", nil)
	req2.Header.Set("Range", "bytes=0-5")
	s.streamFile(rec2, req2, mime, "f.txt", "f.txt")
	if rec2.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rec2.Code)
	}
}
