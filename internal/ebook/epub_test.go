package ebook

import (
	"archive/zip"
	"os"
	"testing"
)

// buildMinimalEPUB writes a valid minimal EPUB to a temp file and returns the path.
// The EPUB contains: mimetype, META-INF/container.xml, OEBPS/content.opf,
// OEBPS/cover.png (a few fake bytes).
func buildMinimalEPUB(t *testing.T) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "test-*.epub")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)

	// mimetype must be the first file and stored (not compressed).
	mw, err := w.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	})
	if err != nil {
		t.Fatal(err)
	}
	mw.Write([]byte("application/epub+zip"))

	// META-INF/container.xml
	cw, err := w.Create("META-INF/container.xml")
	if err != nil {
		t.Fatal(err)
	}
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// OEBPS/content.opf — OPF with all the metadata we want to parse.
	opfw, err := w.Create("OEBPS/content.opf")
	if err != nil {
		t.Fatal(err)
	}
	opfw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0"
  xmlns="http://www.idpf.org/2007/opf"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns:opf="http://www.idpf.org/2007/opf"
  unique-identifier="BookId">
  <metadata>
    <dc:title>The Great Test Book</dc:title>
    <dc:creator opf:file-as="Doe, John" opf:role="aut">John Doe</dc:creator>
    <dc:creator opf:role="aut">Jane Smith</dc:creator>
    <dc:language>en</dc:language>
    <dc:subject>Fiction</dc:subject>
    <dc:subject>Testing</dc:subject>
    <dc:publisher>Test Publisher</dc:publisher>
    <dc:date>2024-01-15</dc:date>
    <dc:description>A book used for unit tests.</dc:description>
    <dc:identifier id="BookId" opf:scheme="ISBN">978-3-16-148410-0</dc:identifier>
    <meta name="calibre:series" content="Test Series"/>
    <meta name="calibre:series_index" content="2.0"/>
    <meta name="cover" content="cover-img"/>
  </metadata>
  <manifest>
    <item id="cover-img" href="cover.png" media-type="image/png" properties="cover-image"/>
    <item id="content" href="content.html" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="content"/>
  </spine>
</package>`))

	// OEBPS/cover.png — a few fake bytes that represent a PNG.
	// A real PNG starts with the 8-byte signature; we use a small stub.
	pw, err := w.Create("OEBPS/cover.png")
	if err != nil {
		t.Fatal(err)
	}
	pw.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")) // 16-byte PNG stub

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestParseEPUB(t *testing.T) {
	epubPath := buildMinimalEPUB(t)

	m, err := parseEPUB(epubPath)
	if err != nil {
		t.Fatalf("parseEPUB returned error: %v", err)
	}

	// Title.
	if got, want := m.Title, "The Great Test Book"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}

	// Authors: two authors, first has file-as sort name.
	if got := len(m.Authors); got != 2 {
		t.Fatalf("len(Authors) = %d, want 2", got)
	}
	if got, want := m.Authors[0].Name, "John Doe"; got != want {
		t.Errorf("Authors[0].Name = %q, want %q", got, want)
	}
	if got, want := m.Authors[0].SortName, "Doe, John"; got != want {
		t.Errorf("Authors[0].SortName = %q, want %q", got, want)
	}
	if got, want := m.Authors[1].Name, "Jane Smith"; got != want {
		t.Errorf("Authors[1].Name = %q, want %q", got, want)
	}
	// No file-as → lowercase of Name.
	if got, want := m.Authors[1].SortName, "jane smith"; got != want {
		t.Errorf("Authors[1].SortName = %q, want %q", got, want)
	}

	// Language.
	if got, want := m.Language, "en"; got != want {
		t.Errorf("Language = %q, want %q", got, want)
	}

	// Tags.
	if got := len(m.Tags); got != 2 {
		t.Fatalf("len(Tags) = %d, want 2", got)
	}
	if m.Tags[0] != "Fiction" || m.Tags[1] != "Testing" {
		t.Errorf("Tags = %v, want [Fiction Testing]", m.Tags)
	}

	// Series.
	if got, want := m.Series, "Test Series"; got != want {
		t.Errorf("Series = %q, want %q", got, want)
	}
	if got, want := m.SeriesIndex, 2.0; got != want {
		t.Errorf("SeriesIndex = %v, want %v", got, want)
	}

	// Cover.
	if len(m.CoverData) == 0 {
		t.Error("CoverData is empty, want non-empty")
	}
	if got, want := m.CoverType, "image/png"; got != want {
		t.Errorf("CoverType = %q, want %q", got, want)
	}

	// Publisher.
	if got, want := m.Publisher, "Test Publisher"; got != want {
		t.Errorf("Publisher = %q, want %q", got, want)
	}

	// ISBN.
	if got, want := m.ISBN, "978-3-16-148410-0"; got != want {
		t.Errorf("ISBN = %q, want %q", got, want)
	}
}

func TestReadMetaEPUB(t *testing.T) {
	epubPath := buildMinimalEPUB(t)

	m, err := ReadMeta(epubPath)
	if err != nil {
		t.Fatalf("ReadMeta returned error: %v", err)
	}

	if got, want := m.Format, "epub"; got != want {
		t.Errorf("Format = %q, want %q", got, want)
	}
	if got, want := m.ContentType, "application/epub+zip"; got != want {
		t.Errorf("ContentType = %q, want %q", got, want)
	}
	if m.Title == "" {
		t.Error("Title is empty")
	}
}

func TestIsBookFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"book.epub", true},
		{"doc.pdf", true},
		{"novel.fb2", true},
		{"comic.cbz", true},
		{"comic.cbr", true},
		{"ebook.mobi", true},
		{"ebook.azw", true},
		{"ebook.azw3", true},
		{"song.mp3", false},
		{"image.jpg", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsBookFile(c.path); got != c.want {
			t.Errorf("IsBookFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestReadMetaTitleFallback(t *testing.T) {
	// Build an EPUB with an empty title.
	f, err := os.CreateTemp(t.TempDir(), "notitle-*.epub")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)

	mw, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/epub+zip"))

	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0"
  xmlns="http://www.idpf.org/2007/opf"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  unique-identifier="BookId">
  <metadata>
    <dc:title></dc:title>
    <dc:language>fr</dc:language>
  </metadata>
  <manifest/>
  <spine/>
</package>`))

	w.Close()

	m, err := ReadMeta(f.Name())
	if err != nil {
		t.Fatalf("ReadMeta error: %v", err)
	}
	// Title must fall back to the filename (without extension).
	base := m.Title
	if base == "" {
		t.Error("Title fallback did not work: got empty string")
	}
}
