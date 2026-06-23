package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
	"discodrive/internal/ebook"
)

// ebookMetaEnv holds everything buildEbookMetaEnv returns.
type ebookMetaEnv struct {
	tok    string
	nid    string
	uid    pgtype.UUID
	s      *Server
	root   string
	folder db.Node
}

// buildEbookMetaEnv creates a user, an ebook folder + settings, a fake epub on
// disk (with a DB node and indexed books row), and a Server with metaEditor.
func buildEbookMetaEnv(t *testing.T, email string) *ebookMetaEnv {
	t.Helper()
	ctx := context.Background()

	_, q, svc := bootstrapPairingDB(t)
	root := t.TempDir()

	tok, user, err := svc.Register(ctx, email, "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := user.ID

	// Ebook folder on disk and in DB.
	booksRelDir := db.UUIDString(uid) + "/ebooks"
	if err := os.MkdirAll(filepath.Join(root, booksRelDir), 0o755); err != nil {
		t.Fatalf("mkdir ebooks: %v", err)
	}
	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: booksRelDir, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode folder: %v", err)
	}

	// Configure ebook_settings so MetadataEditor.resolve() can verify folder membership.
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: folderNode.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	// Place a minimal file in the folder and create a node + books row.
	epubRelPath := booksRelDir + "/test.epub"
	if err := os.WriteFile(filepath.Join(root, epubRelPath), []byte("PK fake epub"), 0o644); err != nil {
		t.Fatalf("write fake epub: %v", err)
	}
	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: folderNode.ID,
		Name:     "test.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: epubRelPath, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode file: %v", err)
	}

	// Insert a books row so MetadataEditor.resolve() finds it.
	if _, err := q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      uid,
		NodeID:      fileNode.ID,
		Title:       "Original Title",
		SortTitle:   "original title",
		Format:      "epub",
		ContentType: "application/epub+zip",
	}); err != nil {
		t.Fatalf("UpsertBook: %v", err)
	}

	ed := ebook.NewMetadataEditor(q, root)
	srv := &Server{auth: svc, q: q, storageRoot: root, metaEditor: ed}
	return &ebookMetaEnv{
		tok:    tok,
		nid:    db.UUIDString(fileNode.ID),
		uid:    uid,
		s:      srv,
		root:   root,
		folder: folderNode,
	}
}

// TestPutEbookMeta_UpdatesTitle: PUT metadata → 204; GET reflects new title + authors.
func TestPutEbookMeta_UpdatesTitle(t *testing.T) {
	env := buildEbookMetaEnv(t, "ebmeta1@x.test")
	tok, nid, s := env.tok, env.nid, env.s

	putH := s.auth.Middleware(http.HandlerFunc(s.handlePutEbookMeta))
	getH := s.auth.Middleware(http.HandlerFunc(s.handleGetEbookMeta))

	body := `{"title":"Новое","authors":["Лев Толстой"],"tags":["классика"],"series":"","seriesIndex":0,"language":"ru","description":"","publisher":"","date":""}`
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodPut, "/me/ebooks/meta/"+nid, tok, body, "")
	req.SetPathValue("id", nid)
	putH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/meta/"+nid, tok, "", "")
	req.SetPathValue("id", nid)
	getH.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"title":"Новое"`) || !strings.Contains(rec.Body.String(), "Лев Толстой") {
		t.Errorf("GET body = %s", rec.Body.String())
	}
}

// TestPostEbookMetaFolder_BulkTags: POST folder bulk → 200, updated==2; GET one book → tag applied.
// Also asserts GET /me/ebooks/library items contain nodeId.
func TestPostEbookMetaFolder_BulkTags(t *testing.T) {
	ctx := context.Background()
	_, q, svc := bootstrapPairingDB(t)
	root := t.TempDir()

	tok, user, err := svc.Register(ctx, "ebfolder@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := user.ID

	// Create ebook folder + settings.
	booksRelDir := db.UUIDString(uid) + "/ebooks"
	if err := os.MkdirAll(filepath.Join(root, booksRelDir), 0o755); err != nil {
		t.Fatalf("mkdir ebooks: %v", err)
	}
	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		Name:     "ebooks",
		IsDir:    true,
		DiskPath: pgtype.Text{String: booksRelDir, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode folder: %v", err)
	}
	if _, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:       uid,
		Enabled:      true,
		FolderNodeID: folderNode.ID,
	}); err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}

	// Create a subfolder.
	subRelDir := booksRelDir + "/manga"
	if err := os.MkdirAll(filepath.Join(root, subRelDir), 0o755); err != nil {
		t.Fatalf("mkdir subfolder: %v", err)
	}
	subFolderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   uid,
		ParentID: folderNode.ID,
		Name:     "manga",
		IsDir:    true,
		DiskPath: pgtype.Text{String: subRelDir, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode subfolder: %v", err)
	}
	subFolderNodeID := db.UUIDString(subFolderNode.ID)

	// Create two book nodes inside the subfolder.
	makeBook := func(name string) (string, string) {
		rel := subRelDir + "/" + name
		_ = os.WriteFile(filepath.Join(root, rel), []byte("PK fake epub"), 0o644)
		fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
			UserID:   uid,
			ParentID: subFolderNode.ID,
			Name:     name,
			IsDir:    false,
			DiskPath: pgtype.Text{String: rel, Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateNode %s: %v", name, err)
		}
		_, err = q.UpsertBook(ctx, db.UpsertBookParams{
			UserID:      uid,
			NodeID:      fileNode.ID,
			Title:       name,
			SortTitle:   name,
			Format:      "epub",
			ContentType: "application/epub+zip",
		})
		if err != nil {
			t.Fatalf("UpsertBook %s: %v", name, err)
		}
		return db.UUIDString(fileNode.ID), db.UUIDString(fileNode.ID)
	}
	book1NodeID, _ := makeBook("book1.epub")
	makeBook("book2.epub")

	ed := ebook.NewMetadataEditor(q, root)
	s := &Server{auth: svc, q: q, storageRoot: root, metaEditor: ed}

	// POST /me/ebooks/bulk/{id} → 200 + updated:2
	body := `{"tags":["манга"],"series":"С"}`
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodPost, "/me/ebooks/bulk/"+subFolderNodeID, tok, body, "")
	req.SetPathValue("id", subFolderNodeID)
	s.auth.Middleware(http.HandlerFunc(s.handlePostEbookMetaFolder)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"updated":2`) {
		t.Fatalf("bulk = %d: %s", rec.Code, rec.Body.String())
	}

	// GET one book's meta → tag applied.
	rec2 := httptest.NewRecorder()
	req2 := authedReq(http.MethodGet, "/me/ebooks/meta/"+book1NodeID, tok, "", "")
	req2.SetPathValue("id", book1NodeID)
	s.auth.Middleware(http.HandlerFunc(s.handleGetEbookMeta)).ServeHTTP(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "манга") {
		t.Errorf("tag not applied: %s", rec2.Body.String())
	}

	// GET /me/ebooks/library → items contain nodeId.
	recList := httptest.NewRecorder()
	s.auth.Middleware(http.HandlerFunc(s.handleListEbooks)).ServeHTTP(recList, authedReq(http.MethodGet, "/me/ebooks/library", tok, "", ""))
	if recList.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", recList.Code, recList.Body.String())
	}
	var listResp ebookListResponse
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v body=%s", err, recList.Body.String())
	}
	for _, b := range listResp.Books {
		if b.NodeID == "" {
			t.Errorf("book %q missing nodeId in library list", b.Title)
		}
	}
}

// TestGetEbookMeta_NonBookNode: a txt file node inside the ebook folder (no books row) → 404.
func TestGetEbookMeta_NonBookNode(t *testing.T) {
	ctx := context.Background()
	env := buildEbookMetaEnv(t, "ebmeta2@x.test")
	tok, s := env.tok, env.s

	// Create a txt file node inside the ebook folder — no books row → ErrNotBook → 404.
	txtRelPath := env.folder.DiskPath.String + "/readme.txt"
	_ = os.WriteFile(filepath.Join(env.root, txtRelPath), []byte("hello"), 0o644)
	txtNode, err := s.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   env.uid,
		ParentID: env.folder.ID,
		Name:     "readme.txt",
		IsDir:    false,
		DiskPath: pgtype.Text{String: txtRelPath, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode txt: %v", err)
	}
	nonBookID := db.UUIDString(txtNode.ID)

	getH := s.auth.Middleware(http.HandlerFunc(s.handleGetEbookMeta))
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodGet, "/me/ebooks/meta/"+nonBookID, tok, "", "")
	req.SetPathValue("id", nonBookID)
	getH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("non-book node: expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}
