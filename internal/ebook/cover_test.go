package ebook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCoverExtMapping(t *testing.T) {
	cases := []struct {
		mime    string
		wantExt string
	}{
		{"image/jpeg", "jpg"},
		{"image/png", "png"},
		{"image/gif", "gif"},
		{"image/webp", "webp"},
		{"image/bmp", "jpg"}, // unknown → defaults to jpg
		{"", "jpg"},          // empty → defaults to jpg
	}

	root := t.TempDir()
	data := []byte("fake image bytes")

	for _, c := range cases {
		rel, err := WriteCover(root, "book-id-1", data, c.mime)
		if err != nil {
			t.Fatalf("WriteCover(%q): %v", c.mime, err)
		}

		want := filepath.Join(".covers", "ebooks", "book-id-1."+c.wantExt)
		if rel != want {
			t.Errorf("mime %q: relPath = %q, want %q", c.mime, rel, want)
		}

		// File must exist on disk.
		abs := filepath.Join(root, rel)
		if _, serr := os.Stat(abs); serr != nil {
			t.Errorf("file not found at %q: %v", abs, serr)
		}
	}
}

func TestWriteCoverRelativePath(t *testing.T) {
	root := t.TempDir()
	rel, err := WriteCover(root, "mybook", []byte("data"), "image/png")
	if err != nil {
		t.Fatalf("WriteCover: %v", err)
	}
	if rel != filepath.Join(".covers", "ebooks", "mybook.png") {
		t.Errorf("relPath = %q, want .covers/ebooks/mybook.png", rel)
	}
	// Absolute path must exist.
	if _, serr := os.Stat(filepath.Join(root, rel)); serr != nil {
		t.Fatalf("cover file missing: %v", serr)
	}
}

func TestWriteCoverOverwrites(t *testing.T) {
	root := t.TempDir()
	first := []byte("first")
	second := []byte("second content longer")

	rel1, err := WriteCover(root, "bk", first, "image/jpeg")
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	rel2, err := WriteCover(root, "bk", second, "image/jpeg")
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if rel1 != rel2 {
		t.Errorf("paths differ on overwrite: %q vs %q", rel1, rel2)
	}

	got, err := os.ReadFile(filepath.Join(root, rel2))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(second) {
		t.Errorf("file content = %q, want %q", got, second)
	}
}

func TestRemoveCover(t *testing.T) {
	root := t.TempDir()
	rel, err := WriteCover(root, "rm-test", []byte("img"), "image/png")
	if err != nil {
		t.Fatalf("WriteCover: %v", err)
	}

	// Remove must succeed.
	if err := RemoveCover(root, rel); err != nil {
		t.Fatalf("RemoveCover: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(serr) {
		t.Errorf("file still exists after RemoveCover")
	}

	// Second remove must be a no-op (no error).
	if err := RemoveCover(root, rel); err != nil {
		t.Errorf("RemoveCover on missing file: %v", err)
	}
}

func TestRemoveCoverEmpty(t *testing.T) {
	// Empty relPath must not error.
	if err := RemoveCover(t.TempDir(), ""); err != nil {
		t.Errorf("RemoveCover(\"\") = %v, want nil", err)
	}
}
