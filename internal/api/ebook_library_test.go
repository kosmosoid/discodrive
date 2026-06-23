package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// seedBook inserts a book row plus optional author/tag rows and returns the book id string.
// coverRel is the relative cover_path; if hasCover is false it is ignored.
func seedBook(
	t *testing.T, ctx context.Context, q *db.Queries,
	uid pgtype.UUID, title, author, series, genre, coverRel string,
	hasCover bool,
) string {
	t.Helper()
	node := createFileNode(t, ctx, q, uid, title+".epub")
	params := db.UpsertBookParams{
		UserID:      uid,
		NodeID:      node.ID,
		Title:       title,
		SortTitle:   title,
		Format:      "epub",
		ContentType: "application/epub+zip",
	}
	if series != "" {
		params.Series = pgtype.Text{String: series, Valid: true}
	}
	book, err := q.UpsertBook(ctx, params)
	if err != nil {
		t.Fatalf("UpsertBook %s: %v", title, err)
	}
	if author != "" {
		if err := q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
			BookID: book.ID, Name: author, SortName: author,
		}); err != nil {
			t.Fatalf("InsertBookAuthor: %v", err)
		}
	}
	if genre != "" {
		if err := q.InsertBookTag(ctx, db.InsertBookTagParams{
			BookID: book.ID, Tag: genre,
		}); err != nil {
			t.Fatalf("InsertBookTag: %v", err)
		}
	}
	if hasCover && coverRel != "" {
		if err := q.SetBookCoverPath(ctx, db.SetBookCoverPathParams{
			ID:        book.ID,
			CoverPath: pgtype.Text{String: coverRel, Valid: true},
		}); err != nil {
			t.Fatalf("SetBookCoverPath: %v", err)
		}
	}
	return db.UUIDString(book.ID)
}

// buildEbookLibraryServer returns a Server with storageRoot set for ebook library tests.
// It reuses buildEbookServer (which runs the real DB migrations).
func buildEbookLibraryServer(t *testing.T) (*db.Queries, *Server) {
	t.Helper()
	q, _, s := buildEbookServer(t)
	s.storageRoot = t.TempDir()
	return q, s
}

func TestEbookLibraryList(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok1, user1, err := svc.Register(ctx, "ebl_list1@x.test", "password12")
	if err != nil {
		t.Fatalf("register user1: %v", err)
	}
	tok2, user2, err := svc.Register(ctx, "ebl_list2@x.test", "password12")
	if err != nil {
		t.Fatalf("register user2: %v", err)
	}

	// Seed user1 books
	seedBook(t, ctx, q, user1.ID, "Alpha Book", "Author A", "Series A", "Fantasy", "", false)
	seedBook(t, ctx, q, user1.ID, "Beta Book", "Author B", "", "Sci-Fi", "", false)
	// Seed user2 book (must NOT appear in user1's list)
	seedBook(t, ctx, q, user2.ID, "User2 Book", "Author C", "", "", "", false)

	listH := svc.Middleware(http.HandlerFunc(s.handleListEbooks))

	// User1 sees only their 2 books
	rec := httptest.NewRecorder()
	listH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library", tok1, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ebookListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(resp.Books) != 2 {
		t.Fatalf("expected 2 books for user1, got %d: %s", len(resp.Books), rec.Body.String())
	}
	for _, b := range resp.Books {
		if b.Title == "User2 Book" {
			t.Fatalf("user2's book leaked into user1's list")
		}
	}

	// User2 sees only their 1 book
	rec = httptest.NewRecorder()
	listH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library", tok2, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 list status=%d", rec.Code)
	}
	var resp2 ebookListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp2)
	if len(resp2.Books) != 1 || resp2.Books[0].Title != "User2 Book" {
		t.Fatalf("user2 list wrong: %s", rec.Body.String())
	}
}

func TestEbookLibraryFilterByAuthor(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_author@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	seedBook(t, ctx, q, user.ID, "Book By A", "Author A", "", "", "", false)
	seedBook(t, ctx, q, user.ID, "Book By B", "Author B", "", "", "", false)

	listH := svc.Middleware(http.HandlerFunc(s.handleListEbooks))
	rec := httptest.NewRecorder()
	listH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library?author=Author+A", tok, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ebookListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Books) != 1 || resp.Books[0].Title != "Book By A" {
		t.Fatalf("author filter wrong: %s", rec.Body.String())
	}
}

func TestEbookLibraryFilterBySeries(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_series@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	seedBook(t, ctx, q, user.ID, "Series Book", "", "My Series", "", "", false)
	seedBook(t, ctx, q, user.ID, "Other Book", "", "", "", "", false)

	listH := svc.Middleware(http.HandlerFunc(s.handleListEbooks))
	rec := httptest.NewRecorder()
	listH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library?series=My+Series", tok, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ebookListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Books) != 1 || resp.Books[0].Title != "Series Book" {
		t.Fatalf("series filter wrong: %s", rec.Body.String())
	}
}

func TestEbookLibraryFilterByGenre(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_genre@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	seedBook(t, ctx, q, user.ID, "Fantasy Book", "", "", "Fantasy", "", false)
	seedBook(t, ctx, q, user.ID, "SciFi Book", "", "", "Sci-Fi", "", false)

	listH := svc.Middleware(http.HandlerFunc(s.handleListEbooks))
	rec := httptest.NewRecorder()
	listH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library?genre=Fantasy", tok, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ebookListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Books) != 1 || resp.Books[0].Title != "Fantasy Book" {
		t.Fatalf("genre filter wrong: %s", rec.Body.String())
	}
}

func TestEbookLibrarySearch(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_search@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	seedBook(t, ctx, q, user.ID, "Rust Programming", "Steve Kl", "", "", "", false)
	seedBook(t, ctx, q, user.ID, "Go in Action", "William K", "", "", "", false)

	listH := svc.Middleware(http.HandlerFunc(s.handleListEbooks))
	rec := httptest.NewRecorder()
	listH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library?q=Rust", tok, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ebookListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Books) != 1 || resp.Books[0].Title != "Rust Programming" {
		t.Fatalf("search wrong: %s", rec.Body.String())
	}
}

func TestEbookLibraryCover(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_cover@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	tok2, user2, err := svc.Register(ctx, "ebl_cover2@x.test", "password12")
	if err != nil {
		t.Fatalf("register user2: %v", err)
	}

	// Write cover file on disk
	coverRel := "covers/ebooks/test-cover.jpg"
	coverAbs := filepath.Join(s.storageRoot, coverRel)
	if err := os.MkdirAll(filepath.Dir(coverAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	coverBytes := []byte("\xff\xd8\xffFAKEJPEG")
	if err := os.WriteFile(coverAbs, coverBytes, 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}

	// User1: book with cover
	bookWithCoverID := seedBook(t, ctx, q, user.ID, "With Cover", "", "", "", coverRel, true)
	// User1: book without cover
	bookNoCoverID := seedBook(t, ctx, q, user.ID, "No Cover", "", "", "", "", false)
	// User2: book with cover (user1 must get 404 for it)
	user2CoverRel := "covers/ebooks/u2cover.jpg"
	_ = os.WriteFile(filepath.Join(s.storageRoot, user2CoverRel), coverBytes, 0o644)
	bookUser2ID := seedBook(t, ctx, q, user2.ID, "User2 Cover Book", "", "", "", user2CoverRel, true)

	coverH := svc.Middleware(http.HandlerFunc(s.handleGetEbookCover))

	// User1 fetches own book cover → 200 + correct bytes
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodGet, "/me/ebooks/library/"+bookWithCoverID+"/cover", tok, "", "")
	req.SetPathValue("id", bookWithCoverID)
	coverH.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cover 200 expected, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), coverBytes) {
		t.Fatalf("cover bytes mismatch")
	}

	// Book without cover → 404
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/library/"+bookNoCoverID+"/cover", tok, "", "")
	req.SetPathValue("id", bookNoCoverID)
	coverH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no-cover book: expected 404, got %d", rec.Code)
	}

	// User2's book cover accessed by user1 → 404 (scope gate, security)
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/library/"+bookUser2ID+"/cover", tok, "", "")
	req.SetPathValue("id", bookUser2ID)
	coverH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user2 book via user1 token: expected 404, got %d", rec.Code)
	}

	// Unknown id → 404
	missing := "00000000-0000-0000-0000-000000000000"
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/library/"+missing+"/cover", tok, "", "")
	req.SetPathValue("id", missing)
	coverH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing id: expected 404, got %d", rec.Code)
	}

	_ = tok2
}

func TestEbookLibraryDownload(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_dl@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	tok2, user2, err := svc.Register(ctx, "ebl_dl2@x.test", "password12")
	if err != nil {
		t.Fatalf("register user2: %v", err)
	}

	// Write a fake epub file to disk.
	epubRel := "user1/books/my-book.epub"
	epubAbs := filepath.Join(s.storageRoot, epubRel)
	if err := os.MkdirAll(filepath.Dir(epubAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	epubBytes := []byte("PK fake epub content")
	if err := os.WriteFile(epubAbs, epubBytes, 0o644); err != nil {
		t.Fatalf("write epub: %v", err)
	}

	// Create a node with the disk_path set at creation time.
	node, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   user.ID,
		Name:     "my-book.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: epubRel, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	book, err := q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user.ID,
		NodeID:      node.ID,
		Title:       "My Book",
		SortTitle:   "My Book",
		Format:      "epub",
		ContentType: "application/epub+zip",
	})
	if err != nil {
		t.Fatalf("UpsertBook: %v", err)
	}
	bookID := db.UUIDString(book.ID)

	// User2's book
	epubRel2 := "user2/books/u2.epub"
	epubAbs2 := filepath.Join(s.storageRoot, epubRel2)
	_ = os.MkdirAll(filepath.Dir(epubAbs2), 0o755)
	_ = os.WriteFile(epubAbs2, epubBytes, 0o644)
	node2, _ := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   user2.ID,
		Name:     "u2book.epub",
		IsDir:    false,
		DiskPath: pgtype.Text{String: epubRel2, Valid: true},
	})
	book2, _ := q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user2.ID,
		NodeID:      node2.ID,
		Title:       "U2 Book",
		SortTitle:   "U2 Book",
		Format:      "epub",
		ContentType: "application/epub+zip",
	})
	bookID2 := db.UUIDString(book2.ID)

	dlH := svc.Middleware(http.HandlerFunc(s.handleDownloadEbook))

	// User1 downloads own book → 200 with epub bytes
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodGet, "/me/ebooks/library/"+bookID+"/download", tok, "", "")
	req.SetPathValue("id", bookID)
	dlH.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), epubBytes) {
		t.Fatalf("download bytes mismatch: got %q", rec.Body.Bytes())
	}

	// Range request → 206
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/library/"+bookID+"/download", tok, "", "")
	req.SetPathValue("id", bookID)
	req.Header.Set("Range", "bytes=0-3")
	dlH.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range download status=%d body=%s", rec.Code, rec.Body.String())
	}

	// User1 tries to download user2's book → 404 (scope gate, security)
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/library/"+bookID2+"/download", tok, "", "")
	req.SetPathValue("id", bookID2)
	dlH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user2 book via user1: expected 404, got %d", rec.Code)
	}

	// Missing book id → 404
	missing := "00000000-0000-0000-0000-000000000000"
	rec = httptest.NewRecorder()
	req = authedReq(http.MethodGet, "/me/ebooks/library/"+missing+"/download", tok, "", "")
	req.SetPathValue("id", missing)
	dlH.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing id: expected 404, got %d", rec.Code)
	}

	_ = tok2
}

func TestEbookLibraryFacets(t *testing.T) {
	ctx := context.Background()
	q, s := buildEbookLibraryServer(t)
	svc := s.auth

	tok, user, err := svc.Register(ctx, "ebl_facets@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, user2, err := svc.Register(ctx, "ebl_facets2@x.test", "password12")
	if err != nil {
		t.Fatalf("register user2: %v", err)
	}

	seedBook(t, ctx, q, user.ID, "Book 1", "Author A", "Series X", "Fantasy", "", false)
	seedBook(t, ctx, q, user.ID, "Book 2", "Author B", "", "Sci-Fi", "", false)
	// User2's book — must not appear in user1's facets
	seedBook(t, ctx, q, user2.ID, "U2 Book", "Author Z", "Series Z", "Horror", "", false)

	facetsH := svc.Middleware(http.HandlerFunc(s.handleEbookFacets))
	rec := httptest.NewRecorder()
	facetsH.ServeHTTP(rec, authedReq(http.MethodGet, "/me/ebooks/library/facets", tok, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("facets status=%d body=%s", rec.Code, rec.Body.String())
	}
	var facets ebookFacetsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &facets); err != nil {
		t.Fatalf("decode facets: %v body=%s", err, rec.Body.String())
	}
	if len(facets.Authors) != 2 {
		t.Fatalf("expected 2 authors, got %d: %v", len(facets.Authors), facets.Authors)
	}
	if len(facets.Genres) != 2 {
		t.Fatalf("expected 2 genres, got %d: %v", len(facets.Genres), facets.Genres)
	}
	// User2's data must not leak
	for _, a := range facets.Authors {
		if a == "Author Z" {
			t.Fatalf("user2 author leaked into user1 facets")
		}
	}
	for _, g := range facets.Genres {
		if g == "Horror" {
			t.Fatalf("user2 genre leaked into user1 facets")
		}
	}
}
