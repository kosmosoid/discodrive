package ebook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// TestMetadataEditor_WriteReadResetAndScanSkip verifies the full lifecycle:
// Write edits → Read returns them (Edited=true) → ScanFolder skips the edited
// book → Reset clears the flag and re-indexes from file metadata.
func TestMetadataEditor_WriteReadResetAndScanSkip(t *testing.T) {
	q, ctx := setupDB(t)
	userID := makeTenant(t, q, ctx)

	storageRoot := t.TempDir()

	uid, _ := db.ParseUUID(userID)

	// Create ebook folder on disk and in the node tree.
	booksDir := filepath.Join(storageRoot, "ebooks")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatalf("mkdir ebooks: %v", err)
	}

	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: "ebooks", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (folder): %v", err)
	}

	// Configure ebook_settings so resolve() knows this is the ebook folder.
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: folderNode.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	// Place a real EPUB inside the folder and create a file node for it.
	epubPath := filepath.Join(booksDir, "book.epub")
	buildEPUBAt(t, epubPath)

	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: folderNode.ID,
		Name:     "book.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "ebooks/book.epub", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (file): %v", err)
	}

	// Index the book so a books row exists.
	ix := NewIndexer(q, storageRoot)
	if err := ix.IndexNode(ctx, userID, db.UUIDString(fileNode.ID), epubPath); err != nil {
		t.Fatalf("IndexNode: %v", err)
	}

	nodeID := db.UUIDString(fileNode.ID)
	folderNodeID := db.UUIDString(folderNode.ID)
	ed := NewMetadataEditor(q, storageRoot)

	// Confirm original title from file before editing.
	orig, err := ed.Read(ctx, userID, nodeID)
	if err != nil {
		t.Fatalf("Read (pre-write): %v", err)
	}
	if orig.Edited {
		t.Error("book should not be edited before Write")
	}
	origTitle := orig.Title

	// --- Write edits ---
	in := BookMeta{
		Title:       "Отредактированное название",
		Authors:     []string{"Автор Один", "Автор Два"},
		Tags:        []string{"фантастика", "роман"},
		Series:      "Моя серия",
		SeriesIndex: 3,
		Language:    "ru",
		Description: "описание",
		Publisher:   "Издатель",
		Date:        "2021",
	}
	if err := ed.Write(ctx, userID, nodeID, in); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// --- Read must return written values with Edited=true ---
	got, err := ed.Read(ctx, userID, nodeID)
	if err != nil {
		t.Fatalf("Read (post-write): %v", err)
	}
	if got.Title != in.Title {
		t.Errorf("Read.Title = %q, want %q", got.Title, in.Title)
	}
	if !got.Edited {
		t.Error("Read.Edited should be true after Write")
	}
	if len(got.Authors) != 2 {
		t.Errorf("Read.Authors = %v, want 2 entries", got.Authors)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Read.Tags = %v, want 2 entries", got.Tags)
	}
	if got.Series != in.Series {
		t.Errorf("Read.Series = %q, want %q", got.Series, in.Series)
	}
	if got.Language != in.Language {
		t.Errorf("Read.Language = %q, want %q", got.Language, in.Language)
	}

	// --- ScanFolder must NOT clobber the edited book ---
	if _, err := ix.ScanFolder(ctx, userID, folderNodeID); err != nil {
		t.Fatalf("ScanFolder: %v", err)
	}
	after, err := ed.Read(ctx, userID, nodeID)
	if err != nil {
		t.Fatalf("Read (post-scan): %v", err)
	}
	if after.Title != in.Title {
		t.Errorf("ScanFolder clobbered edit: title=%q, want %q", after.Title, in.Title)
	}
	if !after.Edited {
		t.Error("ScanFolder cleared Edited flag — should not have")
	}

	// --- Reset must clear the flag and restore file metadata ---
	if err := ed.Reset(ctx, userID, nodeID); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	reset, err := ed.Read(ctx, userID, nodeID)
	if err != nil {
		t.Fatalf("Read (post-reset): %v", err)
	}
	if reset.Edited {
		t.Error("Reset should clear Edited flag")
	}
	if reset.Title == in.Title {
		t.Errorf("Reset should restore file title, still has the edited one: %q", reset.Title)
	}
	if reset.Title != origTitle {
		t.Errorf("Reset.Title = %q, want original %q", reset.Title, origTitle)
	}
}

// TestMetadataEditor_ErrNotBook verifies that Read/Write on a non-book node
// (directory, or file not indexed as a book) returns ErrNotBook.
func TestMetadataEditor_ErrNotBook(t *testing.T) {
	q, ctx := setupDB(t)
	userID := makeTenant(t, q, ctx)

	storageRoot := t.TempDir()
	uid, _ := db.ParseUUID(userID)

	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: "ebooks", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (folder): %v", err)
	}
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: folderNode.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	// A plain file node inside the folder but never indexed (no books row).
	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: folderNode.ID,
		Name:     "readme.txt",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "ebooks/readme.txt", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (file): %v", err)
	}

	ed := NewMetadataEditor(q, storageRoot)
	nodeID := db.UUIDString(fileNode.ID)

	if _, err := ed.Read(ctx, userID, nodeID); err == nil {
		t.Error("Read on non-book node should return error")
	}
}

// TestMetadataEditor_ErrNotInEbookFolder verifies that a node outside the
// configured ebook folder is rejected with ErrNotInEbookFolder.
func TestMetadataEditor_ErrNotInEbookFolder(t *testing.T) {
	q, ctx := setupDB(t)
	userID := makeTenant(t, q, ctx)

	storageRoot := t.TempDir()
	uid, _ := db.ParseUUID(userID)

	// Ebook folder.
	ebookFolder, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: "ebooks", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (ebook folder): %v", err)
	}
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: ebookFolder.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	// A file node outside the ebook folder.
	outsideNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "other.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "other/other.epub", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (outside file): %v", err)
	}

	// Index this file as a book so GetBookByNode finds it (tests folder check specifically).
	epubPath := filepath.Join(storageRoot, "other", "other.epub")
	if err := os.MkdirAll(filepath.Dir(epubPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	buildEPUBAt(t, epubPath)

	ix := NewIndexer(q, storageRoot)
	if err := ix.IndexNode(ctx, userID, db.UUIDString(outsideNode.ID), epubPath); err != nil {
		t.Fatalf("IndexNode: %v", err)
	}

	ed := NewMetadataEditor(q, storageRoot)
	_, err = ed.Read(ctx, userID, db.UUIDString(outsideNode.ID))
	if err == nil {
		t.Error("Read on out-of-folder node should return error")
	}
}

// TestMetadataEditor_WriteFolderBulk verifies folder-bulk editing: CountFolderBooks
// returns the right count, WriteFolder applies requested fields to all books while
// preserving untouched fields, and sets Edited=true on each.
func TestMetadataEditor_WriteFolderBulk(t *testing.T) {
	q, ctx := setupDB(t)
	userID := makeTenant(t, q, ctx)

	storageRoot := t.TempDir()
	uid, _ := db.ParseUUID(userID)

	// Create the root ebook folder.
	ebooksDir := filepath.Join(storageRoot, "ebooks")
	if err := os.MkdirAll(ebooksDir, 0o755); err != nil {
		t.Fatalf("mkdir ebooks: %v", err)
	}
	ebookFolder, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: "ebooks", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (ebook folder): %v", err)
	}
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: ebookFolder.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	// Create a subfolder "manga" inside the ebook folder.
	mangaDir := filepath.Join(ebooksDir, "manga")
	if err := os.MkdirAll(mangaDir, 0o755); err != nil {
		t.Fatalf("mkdir manga: %v", err)
	}
	mangaFolder, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: ebookFolder.ID,
		Name:     "manga",
		IsDir:    true,
		DiskPath: pgtype.Text{String: "ebooks/manga", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (manga folder): %v", err)
	}
	subFolderNodeID := db.UUIDString(mangaFolder.ID)

	// Place two EPUBs in the subfolder and index them.
	ix := NewIndexer(q, storageRoot)

	xPath := filepath.Join(mangaDir, "x.epub")
	buildEPUBAt(t, xPath)
	xNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: mangaFolder.ID,
		Name:     "x.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "ebooks/manga/x.epub", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (x.epub): %v", err)
	}
	if err := ix.IndexNode(ctx, userID, db.UUIDString(xNode.ID), xPath); err != nil {
		t.Fatalf("IndexNode x.epub: %v", err)
	}

	yPath := filepath.Join(mangaDir, "y.epub")
	buildEPUBAt(t, yPath)
	yNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: mangaFolder.ID,
		Name:     "y.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "ebooks/manga/y.epub", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (y.epub): %v", err)
	}
	if err := ix.IndexNode(ctx, userID, db.UUIDString(yNode.ID), yPath); err != nil {
		t.Fatalf("IndexNode y.epub: %v", err)
	}

	// Give the two books distinct titles so we can assert title preservation.
	ed := NewMetadataEditor(q, storageRoot)
	xNodeID := db.UUIDString(xNode.ID)
	yNodeID := db.UUIDString(yNode.ID)
	if err := ed.Write(ctx, userID, xNodeID, BookMeta{Title: "Book X", Authors: []string{"Author X"}}); err != nil {
		t.Fatalf("Write x: %v", err)
	}
	if err := ed.Write(ctx, userID, yNodeID, BookMeta{Title: "Book Y", Authors: []string{"Author Y"}}); err != nil {
		t.Fatalf("Write y: %v", err)
	}

	// CountFolderBooks must return 2.
	n, err := ed.CountFolderBooks(ctx, userID, subFolderNodeID)
	if err != nil || n != 2 {
		t.Fatalf("CountFolderBooks = %d, %v; want 2", n, err)
	}

	// WriteFolder: apply Tags + Series only (leave Title/Authors untouched).
	series := "Манга"
	res, err := ed.WriteFolder(ctx, userID, subFolderNodeID, BulkInput{
		Tags:   []string{"манга", "сёнэн"},
		Series: &series,
	})
	if err != nil {
		t.Fatalf("WriteFolder: %v", err)
	}
	if res.Affected != 2 || res.Updated != 2 || len(res.Failed) != 0 {
		t.Fatalf("res = %+v, want Affected=2 Updated=2 Failed=[]", res)
	}

	// Both books must have new tags + series, Edited=true, and original titles/authors preserved.
	wantTitles := map[string]string{xNodeID: "Book X", yNodeID: "Book Y"}
	wantAuthors := map[string]string{xNodeID: "Author X", yNodeID: "Author Y"}
	for _, nid := range []string{xNodeID, yNodeID} {
		m, err := ed.Read(ctx, userID, nid)
		if err != nil {
			t.Fatalf("Read %s: %v", nid, err)
		}
		if len(m.Tags) != 2 || m.Series != "Манга" {
			t.Errorf("book %s: tags=%v series=%q; want 2 tags and series=Манга", nid, m.Tags, m.Series)
		}
		if !m.Edited {
			t.Errorf("book %s: Edited not set after WriteFolder", nid)
		}
		if m.Title != wantTitles[nid] {
			t.Errorf("book %s: Title=%q, want %q (should be preserved)", nid, m.Title, wantTitles[nid])
		}
		if len(m.Authors) != 1 || m.Authors[0] != wantAuthors[nid] {
			t.Errorf("book %s: Authors=%v, want [%s] (should be preserved)", nid, m.Authors, wantAuthors[nid])
		}
	}
}

// TestMetadataEditor_EmptyAuthorsAndTagsSkipped verifies that Write trims and
// skips empty author/tag strings.
func TestMetadataEditor_EmptyAuthorsAndTagsSkipped(t *testing.T) {
	q, ctx := setupDB(t)
	userID := makeTenant(t, q, ctx)

	storageRoot := t.TempDir()
	uid, _ := db.ParseUUID(userID)

	booksDir := filepath.Join(storageRoot, "ebooks")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: "ebooks", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (folder): %v", err)
	}
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: folderNode.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	epubPath := filepath.Join(booksDir, "book2.epub")
	buildEPUBAt(t, epubPath)

	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: folderNode.ID,
		Name:     "book2.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: "ebooks/book2.epub", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (file): %v", err)
	}

	ix := NewIndexer(q, storageRoot)
	if err := ix.IndexNode(ctx, userID, db.UUIDString(fileNode.ID), epubPath); err != nil {
		t.Fatalf("IndexNode: %v", err)
	}

	ed := NewMetadataEditor(q, storageRoot)
	nodeID := db.UUIDString(fileNode.ID)

	in := BookMeta{
		Title:   "Clean Title",
		Authors: []string{"Good Author", "", "  "},
		Tags:    []string{"tag1", "", "  "},
	}
	if err := ed.Write(ctx, userID, nodeID, in); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := ed.Read(ctx, userID, nodeID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Authors) != 1 {
		t.Errorf("Authors = %v, want 1 (empty/whitespace skipped)", got.Authors)
	}
	if len(got.Tags) != 1 {
		t.Errorf("Tags = %v, want 1 (empty/whitespace skipped)", got.Tags)
	}
}
