package ebook

import (
	"os"
	"testing"
)

// minimalPDFWithInfo is a minimal valid PDF with an Info dictionary containing
// Title and Author entries.  Hand-crafted to avoid a pdfcpu dependency on
// the create-API.
//
// NOTE: the xref offsets below are approximate; pdfcpu's ValidationRelaxed
// mode recovers objects via a linear scan when xref entries don't resolve,
// so these fixtures parse despite imperfect offsets.
//
// Structure:
//   1 0 obj  — catalog
//   2 0 obj  — pages
//   3 0 obj  — page
//   4 0 obj  — info dict  (Title, Author)
//   xref + trailer
//
// PDF string encoding: plain ASCII parentheses literal, no special chars.
var minimalPDFWithInfo = []byte(
	"%PDF-1.4\n" +
		"1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n" +
		"3 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n" +
		"4 0 obj\n<</Title (Go Programming Book) /Author (Rob Pike)>>\nendobj\n" +
		"xref\n" +
		"0 5\n" +
		"0000000000 65535 f \n" +
		"0000000009 00000 n \n" +
		"0000000058 00000 n \n" +
		"0000000115 00000 n \n" +
		"0000000206 00000 n \n" +
		"trailer\n<</Size 5 /Root 1 0 R /Info 4 0 R>>\n" +
		"startxref\n" +
		"270\n" +
		"%%EOF\n")

// minimalPDFNoInfo is a minimal valid PDF with no Info dictionary.
var minimalPDFNoInfo = []byte(
	"%PDF-1.4\n" +
		"1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n" +
		"3 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n" +
		"xref\n" +
		"0 4\n" +
		"0000000000 65535 f \n" +
		"0000000009 00000 n \n" +
		"0000000058 00000 n \n" +
		"0000000115 00000 n \n" +
		"trailer\n<</Size 4 /Root 1 0 R>>\n" +
		"startxref\n" +
		"206\n" +
		"%%EOF\n")

func writeTempPDF(t *testing.T, data []byte, nameSuffix string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*"+nameSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestParsePDF_WithInfo(t *testing.T) {
	path := writeTempPDF(t, minimalPDFWithInfo, ".pdf")

	m, err := parsePDF(path)
	if err != nil {
		t.Fatalf("parsePDF: unexpected error: %v", err)
	}

	if m.Title != "Go Programming Book" {
		t.Errorf("Title = %q; want %q", m.Title, "Go Programming Book")
	}
	if len(m.Authors) != 1 {
		t.Fatalf("Authors len = %d; want 1", len(m.Authors))
	}
	if m.Authors[0].Name != "Rob Pike" {
		t.Errorf("Author.Name = %q; want %q", m.Authors[0].Name, "Rob Pike")
	}
	if m.Authors[0].SortName != "rob pike" {
		t.Errorf("Author.SortName = %q; want %q", m.Authors[0].SortName, "rob pike")
	}
	if m.CoverData != nil {
		t.Error("CoverData should be nil for PDFs")
	}
}

func TestParsePDF_EmptyInfo(t *testing.T) {
	path := writeTempPDF(t, minimalPDFNoInfo, ".pdf")

	m, err := parsePDF(path)
	if err != nil {
		t.Fatalf("parsePDF: unexpected error: %v", err)
	}
	// Title is empty from parse; ReadMeta will fill it from the filename.
	if m.Title != "" {
		t.Errorf("Title = %q; want empty string", m.Title)
	}
	if len(m.Authors) != 0 {
		t.Errorf("Authors = %v; want empty", m.Authors)
	}
}

func TestParsePDF_Malformed(t *testing.T) {
	path := writeTempPDF(t, []byte("this is not a pdf at all"), ".pdf")

	_, err := parsePDF(path)
	if err == nil {
		t.Fatal("parsePDF: expected error for malformed PDF, got nil")
	}
}

func TestReadMeta_PDF_FilenameFallback(t *testing.T) {
	// Write a PDF with no title to a temp file whose name we control via
	// the ReadMeta call — the name is used as the fallback title.
	dir := t.TempDir()
	path := dir + "/my-great-book.pdf"
	if err := os.WriteFile(path, minimalPDFNoInfo, 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := ReadMeta(path)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.Title != "my-great-book" {
		t.Errorf("Title = %q; want %q", meta.Title, "my-great-book")
	}
	if meta.Format != "pdf" {
		t.Errorf("Format = %q; want %q", meta.Format, "pdf")
	}
	if meta.ContentType != "application/pdf" {
		t.Errorf("ContentType = %q; want %q", meta.ContentType, "application/pdf")
	}
}
