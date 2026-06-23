package ebook

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// sampleFB2XML returns a minimal but complete FB2 XML document for testing.
// It includes two authors, a sequence, genres, annotation, date, and a cover image.
func sampleFB2XML() []byte {
	// The cover binary is a 1x1 white JPEG encoded as base64.
	// Generated with: python3 -c "import base64; ..."
	// This is a valid minimal JPEG (1x1 white pixel).
	const coverBase64 = "/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8U" +
		"HRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgN" +
		"DRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIy" +
		"MjL/wAARCAABAAEDASIAAhEBAxEB/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAA" +
		"AAAAAAAAAAAAAP/EABQBAQAAAAAAAAAAAAAAAAAAAAD/xAAUEQEAAAAAAAAAAAAAAAAAAAAA" +
		"/9oADAMBAAIRAxEAPwCwABmX/9k="

	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0"
             xmlns:l="http://www.w3.org/1999/xlink">
  <description>
    <title-info>
      <genre>adventure</genre>
      <genre>fantasy</genre>
      <author>
        <first-name>John</first-name>
        <last-name>Tolkien</last-name>
      </author>
      <author>
        <nickname>TheEditor</nickname>
      </author>
      <book-title>The Fellowship of the Ring</book-title>
      <annotation>
        <p>A young hobbit embarks on an epic quest.</p>
      </annotation>
      <date>1954</date>
      <coverpage>
        <image l:href="#cover.jpg"/>
      </coverpage>
      <lang>en</lang>
      <sequence name="The Lord of the Rings" number="3"/>
    </title-info>
  </description>
  <binary id="cover.jpg" content-type="image/jpeg">` + coverBase64 + `</binary>
</FictionBook>`)
}

func TestParseFB2_Basic(t *testing.T) {
	// Write a temp .fb2 file.
	dir := t.TempDir()
	fb2Path := filepath.Join(dir, "test.fb2")
	if err := os.WriteFile(fb2Path, sampleFB2XML(), 0644); err != nil {
		t.Fatalf("write fb2: %v", err)
	}

	m, err := parseFB2(fb2Path)
	if err != nil {
		t.Fatalf("parseFB2: %v", err)
	}

	// Title
	if m.Title != "The Fellowship of the Ring" {
		t.Errorf("Title: got %q, want %q", m.Title, "The Fellowship of the Ring")
	}

	// Authors: two — one with first+last, one nickname-only
	if len(m.Authors) != 2 {
		t.Fatalf("Authors: got %d, want 2", len(m.Authors))
	}
	if m.Authors[0].Name != "John Tolkien" {
		t.Errorf("Authors[0].Name: got %q, want %q", m.Authors[0].Name, "John Tolkien")
	}
	if m.Authors[0].SortName != "tolkien john" {
		t.Errorf("Authors[0].SortName: got %q, want %q", m.Authors[0].SortName, "tolkien john")
	}
	if m.Authors[1].Name != "TheEditor" {
		t.Errorf("Authors[1].Name: got %q, want %q", m.Authors[1].Name, "TheEditor")
	}
	if m.Authors[1].SortName != "theeditor" {
		t.Errorf("Authors[1].SortName: got %q, want %q", m.Authors[1].SortName, "theeditor")
	}

	// Language
	if m.Language != "en" {
		t.Errorf("Language: got %q, want %q", m.Language, "en")
	}

	// Series
	if m.Series != "The Lord of the Rings" {
		t.Errorf("Series: got %q, want %q", m.Series, "The Lord of the Rings")
	}
	if m.SeriesIndex != 3 {
		t.Errorf("SeriesIndex: got %v, want 3", m.SeriesIndex)
	}

	// Tags
	if len(m.Tags) != 2 {
		t.Errorf("Tags: got %v, want [adventure fantasy]", m.Tags)
	}

	// Description
	if m.Description == "" {
		t.Error("Description: should not be empty")
	}

	// Date
	if m.Date != "1954" {
		t.Errorf("Date: got %q, want %q", m.Date, "1954")
	}

	// Cover: must have data and correct MIME type.
	// This also verifies the XLink namespace href binding works.
	if len(m.CoverData) == 0 {
		t.Error("CoverData: empty — XLink href may not have bound correctly")
	}
	if m.CoverType != "image/jpeg" {
		t.Errorf("CoverType: got %q, want %q", m.CoverType, "image/jpeg")
	}
}

func TestParseFB2_ViaReadMeta_FB2Zip(t *testing.T) {
	// Write the FB2 XML into a .fb2.zip archive.
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.fb2.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("test.fb2")
	if err != nil {
		t.Fatalf("zip create entry: %v", err)
	}
	if _, err := w.Write(sampleFB2XML()); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file close: %v", err)
	}

	// Use ReadMeta (the dispatcher) so we also test .fb2.zip routing.
	m, err := ReadMeta(zipPath)
	if err != nil {
		t.Fatalf("ReadMeta(.fb2.zip): %v", err)
	}

	if m.Title != "The Fellowship of the Ring" {
		t.Errorf("Title via zip: got %q", m.Title)
	}
	if m.Series != "The Lord of the Rings" {
		t.Errorf("Series via zip: got %q", m.Series)
	}
	if m.SeriesIndex != 3 {
		t.Errorf("SeriesIndex via zip: got %v", m.SeriesIndex)
	}
	if m.Format != "fb2.zip" {
		t.Errorf("Format: got %q, want %q", m.Format, "fb2.zip")
	}
}
