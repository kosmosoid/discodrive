package tagwrite

import (
	"os"
	"path/filepath"
	"testing"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

// copyToTemp copies a testdata file into t.TempDir and returns the temp path.
func copyToTemp(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return dst
}

func TestMP3_WriteThenReadRoundTrip(t *testing.T) {
	path := copyToTemp(t, "sample.mp3")
	w, ok := For("mp3")
	if !ok {
		t.Fatal("mp3 must be writable")
	}
	err := w.Apply(path, Tags{
		Title:       strp("New Title"),
		Artist:      strp("New Artist"),
		Album:       strp("New Album"),
		AlbumArtist: strp("New AlbumArtist"),
		Genre:       strp("Jazz"),
		Year:        intp(2021),
		Track:       intp(3),
		Disc:        intp(2),
	}, CoverKeep, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _, err := w.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Title == nil || *got.Title != "New Title" {
		t.Errorf("Title = %v, want New Title", got.Title)
	}
	if got.Artist == nil || *got.Artist != "New Artist" {
		t.Errorf("Artist = %v, want New Artist", got.Artist)
	}
	if got.Year == nil || *got.Year != 2021 {
		t.Errorf("Year = %v, want 2021", got.Year)
	}
	if got.Track == nil || *got.Track != 3 {
		t.Errorf("Track = %v, want 3", got.Track)
	}
}

// writeV23Fixture builds a minimal ID3v2.3 mp3 whose parsed default encoding is
// ISO-8859-1 — the condition real-world v2.3 files exhibit (id3v2 defaults v2.3
// tags to ISO, v2.4 to UTF-8). This is what makes the Cyrillic write fail
// without the encoding fix.
func writeV23Fixture(t *testing.T) string {
	t.Helper()
	// One ISO TIT2 frame: "TIT2" + size(uint32 BE=2) + flags(2) + [ISO byte, 'X'].
	frame := []byte{'T', 'I', 'T', '2', 0, 0, 0, 2, 0, 0, 0x00, 'X'}
	size := len(frame) // 12 → synchsafe is identical for values < 128
	header := []byte{'I', 'D', '3', 3, 0, 0,
		byte((size >> 21) & 0x7f), byte((size >> 14) & 0x7f), byte((size >> 7) & 0x7f), byte(size & 0x7f)}
	data := append(append(header, frame...), 0xFF, 0xFB, 0x90, 0x00) // + a fake MPEG frame
	dst := filepath.Join(t.TempDir(), "v23.mp3")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write v23 fixture: %v", err)
	}
	return dst
}

// TestMP3_CyrillicTags guards against the ISO-8859-1 default-encoding bug:
// non-Latin text must be written with a Unicode encoding, not silently fail.
func TestMP3_CyrillicTags(t *testing.T) {
	path := writeV23Fixture(t)
	w, _ := For("mp3")
	err := w.Apply(path, Tags{
		Artist: strp("Би-2"),
		Album:  strp("Путешествие вокруг Солнца"),
		Title:  strp("Свободная любовь"),
	}, CoverKeep, nil)
	if err != nil {
		t.Fatalf("Apply cyrillic: %v", err)
	}
	got, _, err := w.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Artist == nil || *got.Artist != "Би-2" {
		t.Errorf("Artist = %v, want Би-2", got.Artist)
	}
	if got.Album == nil || *got.Album != "Путешествие вокруг Солнца" {
		t.Errorf("Album = %v, want Путешествие вокруг Солнца", got.Album)
	}
	if got.Title == nil || *got.Title != "Свободная любовь" {
		t.Errorf("Title = %v, want Свободная любовь", got.Title)
	}
}

func TestMP3_CoverReplaceAndRemove(t *testing.T) {
	path := copyToTemp(t, "sample.mp3")
	w, _ := For("mp3")
	png := []byte("\x89PNG\r\n\x1a\n") // header is enough for id3v2 APIC
	if err := w.Apply(path, Tags{}, CoverReplace, &Cover{Data: png, Mime: "image/png"}); err != nil {
		t.Fatalf("Apply replace: %v", err)
	}
	if _, _, ok := w.Cover(path); !ok {
		t.Fatal("expected a cover after replace")
	}
	if err := w.Apply(path, Tags{}, CoverRemove, nil); err != nil {
		t.Fatalf("Apply remove: %v", err)
	}
	if _, _, ok := w.Cover(path); ok {
		t.Fatal("expected no cover after remove")
	}
}
