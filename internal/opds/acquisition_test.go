package opds

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

const user2Email = "other@x.test"

// seedAcqBooks seeds user1 with four books (one with a cover, the rest without,
// covering authors/series/genres) plus a second user owning one un-shared book.
// It returns the IDs of user1's books and the id of user2's hidden book.
func seedAcqBooks(t *testing.T, h *Handler) (b1, b2, b3, b4 db.Book, hidden db.Book) {
	t.Helper()
	ctx := t.Context()

	user1, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail user1: %v", err)
	}

	mkNode := func(name string) pgtype.UUID {
		n, err := h.q.CreateNode(ctx, db.CreateNodeParams{
			UserID:   user1.ID,
			ParentID: pgtype.UUID{Valid: false},
			Name:     name,
			IsDir:    true,
		})
		if err != nil {
			t.Fatalf("CreateNode %s: %v", name, err)
		}
		return n.ID
	}

	// Book 1: author Ada Lovelace, series History, genre Science, WITH cover.
	b1, err = h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user1.ID,
		NodeID:      mkNode("n1"),
		Title:       "Book One",
		SortTitle:   "book one",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Language:    pgtype.Text{String: "en", Valid: true},
		Series:      pgtype.Text{String: "History", Valid: true},
		SeriesIndex: pgtype.Float4{Float32: 1, Valid: true},
		Description: pgtype.Text{String: "first book", Valid: true},
		Size:        pgtype.Int8{Int64: 1000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook b1: %v", err)
	}
	if err := h.q.SetBookCoverPath(ctx, db.SetBookCoverPathParams{
		ID:        b1.ID,
		CoverPath: pgtype.Text{String: ".covers/ebooks/b1.jpg", Valid: true},
	}); err != nil {
		t.Fatalf("SetBookCoverPath b1: %v", err)
	}
	// Re-read so b1 carries the cover_path for any direct assertions.
	b1, err = h.q.GetBookByNode(ctx, b1.NodeID)
	if err != nil {
		t.Fatalf("GetBookByNode b1: %v", err)
	}
	if err := h.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID: b1.ID, Name: "Ada Lovelace", SortName: "lovelace, ada",
	}); err != nil {
		t.Fatalf("InsertBookAuthor b1: %v", err)
	}
	if err := h.q.InsertBookTag(ctx, db.InsertBookTagParams{BookID: b1.ID, Tag: "Science"}); err != nil {
		t.Fatalf("InsertBookTag b1: %v", err)
	}

	// Book 2: author Isaac Asimov, series Foundation, NO cover.
	b2, err = h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user1.ID,
		NodeID:      mkNode("n2"),
		Title:       "Book Two",
		SortTitle:   "book two",
		Format:      "pdf",
		ContentType: "application/pdf",
		Series:      pgtype.Text{String: "Foundation", Valid: true},
		SeriesIndex: pgtype.Float4{Float32: 1, Valid: true},
		Size:        pgtype.Int8{Int64: 2000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook b2: %v", err)
	}
	if err := h.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID: b2.ID, Name: "Isaac Asimov", SortName: "asimov, isaac",
	}); err != nil {
		t.Fatalf("InsertBookAuthor b2: %v", err)
	}

	// Book 3: same author as b1 (Ada Lovelace), NO cover.
	b3, err = h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user1.ID,
		NodeID:      mkNode("n3"),
		Title:       "Book Three",
		SortTitle:   "book three",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Size:        pgtype.Int8{Int64: 3000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook b3: %v", err)
	}
	if err := h.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID: b3.ID, Name: "Ada Lovelace", SortName: "lovelace, ada",
	}); err != nil {
		t.Fatalf("InsertBookAuthor b3: %v", err)
	}

	// Book 4: no author/series/genre, NO cover.
	b4, err = h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user1.ID,
		NodeID:      mkNode("n4"),
		Title:       "Book Four",
		SortTitle:   "book four",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Size:        pgtype.Int8{Int64: 4000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook b4: %v", err)
	}

	// Second user with one book that is NOT shared with user1.
	tenant, err := h.q.CreateTenant(ctx, "test2")
	if err != nil {
		t.Fatalf("CreateTenant user2: %v", err)
	}
	user2, err := h.q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        user2Email,
		PasswordHash: "irrelevant",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser user2: %v", err)
	}
	hiddenNode, err := h.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   user2.ID,
		ParentID: pgtype.UUID{Valid: false},
		Name:     "hidden",
		IsDir:    true,
	})
	if err != nil {
		t.Fatalf("CreateNode hidden: %v", err)
	}
	hidden, err = h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      user2.ID,
		NodeID:      hiddenNode.ID,
		Title:       "Secret Book",
		SortTitle:   "secret book",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Size:        pgtype.Int8{Int64: 9000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook hidden: %v", err)
	}
	if err := h.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID: hidden.ID, Name: "Ada Lovelace", SortName: "lovelace, ada",
	}); err != nil {
		t.Fatalf("InsertBookAuthor hidden: %v", err)
	}

	return b1, b2, b3, b4, hidden
}

// findEntry returns the entry whose title matches, or nil.
func findEntry(feed atomFeed, title string) *atomEntry {
	for i := range feed.Entries {
		if feed.Entries[i].Title == title {
			return &feed.Entries[i]
		}
	}
	return nil
}

// hasLink reports whether links contains one with the given rel and an href
// matching want.
func hasLink(links []atomLink, rel, wantHref string) bool {
	for _, l := range links {
		if l.Rel == rel && l.Href == wantHref {
			return true
		}
	}
	return false
}

// TestAcquisitionAllAtom verifies /opds/all (Atom) scopes to user1 with correct
// acquisition links, cover links only on the book that has one, and excludes
// user2's book.
func TestAcquisitionAllAtom(t *testing.T) {
	h, _ := setupOPDS(t)
	b1, b2, _, _, _ := seedAcqBooks(t, h)

	rec := opdsGetBasic(h, "/opds/all", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, mediaTypeAcquisition) {
		t.Errorf("Content-Type=%q, want contains %q", ct, mediaTypeAcquisition)
	}

	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rec.Body.String())
	}

	if len(feed.Entries) != 4 {
		t.Fatalf("expected 4 entries (user1 books), got %d", len(feed.Entries))
	}

	// user2's book must be absent.
	if e := findEntry(feed, "Secret Book"); e != nil {
		t.Error("user2's book leaked into user1's feed")
	}

	// Book One: acquisition link with epub type + both cover links.
	one := findEntry(feed, "Book One")
	if one == nil {
		t.Fatal("missing Book One")
	}
	wantAcq := "/opds/download/" + db.UUIDString(b1.ID)
	if !hasLink(one.Links, "http://opds-spec.org/acquisition", wantAcq) {
		t.Errorf("Book One missing acquisition link %q; got %+v", wantAcq, one.Links)
	}
	for _, l := range one.Links {
		if l.Rel == "http://opds-spec.org/acquisition" && l.Type != "application/epub+zip" {
			t.Errorf("Book One acquisition type=%q, want application/epub+zip", l.Type)
		}
	}
	if !hasLink(one.Links, "http://opds-spec.org/image", "/opds/cover/"+db.UUIDString(b1.ID)) {
		t.Error("Book One missing image link")
	}
	if !hasLink(one.Links, "http://opds-spec.org/image/thumbnail", "/opds/cover/"+db.UUIDString(b1.ID)+"/thumbnail") {
		t.Error("Book One missing thumbnail link")
	}
	// Author element present.
	if len(one.Authors) != 1 || one.Authors[0].Name != "Ada Lovelace" {
		t.Errorf("Book One authors=%+v, want [Ada Lovelace]", one.Authors)
	}

	// Book Two: acquisition link with pdf type + NO cover links.
	two := findEntry(feed, "Book Two")
	if two == nil {
		t.Fatal("missing Book Two")
	}
	wantAcq2 := "/opds/download/" + db.UUIDString(b2.ID)
	if !hasLink(two.Links, "http://opds-spec.org/acquisition", wantAcq2) {
		t.Errorf("Book Two missing acquisition link %q", wantAcq2)
	}
	for _, l := range two.Links {
		if l.Rel == "http://opds-spec.org/acquisition" && l.Type != "application/pdf" {
			t.Errorf("Book Two acquisition type=%q, want application/pdf", l.Type)
		}
		if l.Rel == "http://opds-spec.org/image" || l.Rel == "http://opds-spec.org/image/thumbnail" {
			t.Error("Book Two should have no cover links")
		}
	}
}

// TestAcquisitionAllJSON verifies /opds/all in OPDS 2.0 JSON: same scope, with
// publication links + images correct.
func TestAcquisitionAllJSON(t *testing.T) {
	h, _ := setupOPDS(t)
	b1, _, _, _, _ := seedAcqBooks(t, h)

	rec := opdsGetJSON(h, "/opds/all", testEmail, testPassword)
	f := assertJSONFeed(t, rec)

	if len(f.Publications) != 4 {
		t.Fatalf("expected 4 publications, got %d", len(f.Publications))
	}

	var one *publication2
	for i := range f.Publications {
		if f.Publications[i].Metadata.Title == "Book One" {
			one = &f.Publications[i]
		}
		if f.Publications[i].Metadata.Title == "Secret Book" {
			t.Error("user2's book leaked into JSON feed")
		}
	}
	if one == nil {
		t.Fatal("missing Book One publication")
	}

	wantAcq := "/opds/download/" + db.UUIDString(b1.ID)
	var hasAcq bool
	for _, l := range one.Links {
		if l.Rel == "http://opds-spec.org/acquisition" && l.Href == wantAcq && l.Type == "application/epub+zip" {
			hasAcq = true
		}
	}
	if !hasAcq {
		t.Errorf("Book One JSON missing acquisition link %q; got %+v", wantAcq, one.Links)
	}
	if len(one.Images) != 2 {
		t.Errorf("Book One JSON images=%+v, want 2 (cover + thumbnail)", one.Images)
	}
	if one.Metadata.Identifier != "urn:discodrive:book:"+db.UUIDString(b1.ID) {
		t.Errorf("Book One identifier=%q", one.Metadata.Identifier)
	}
	if len(one.Metadata.Author) != 1 || one.Metadata.Author[0].Name != "Ada Lovelace" {
		t.Errorf("Book One author=%+v, want [Ada Lovelace]", one.Metadata.Author)
	}
}

// TestAcquisitionByAuthor verifies /opds/author/{escaped} returns only that
// author's accessible books (and not the same-named author's book owned by user2).
func TestAcquisitionByAuthor(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	rec := opdsGetBasic(h, "/opds/author/"+encodeNameID("Ada Lovelace"), testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Ada Lovelace wrote Book One and Book Three (user1), plus the hidden user2 book.
	if len(feed.Entries) != 2 {
		t.Fatalf("expected 2 entries for Ada Lovelace, got %d", len(feed.Entries))
	}
	titles := map[string]bool{}
	for _, e := range feed.Entries {
		titles[e.Title] = true
	}
	if !titles["Book One"] || !titles["Book Three"] {
		t.Errorf("expected Book One + Book Three, got %v", titles)
	}
	if titles["Secret Book"] {
		t.Error("user2's same-author book leaked")
	}
}

// TestAcquisitionPagination verifies rel=next behavior across pages with a small
// pageSize. user1 has 4 accessible books; pageSize=2 → page1 has next, page2
// (?start=2) also has next? No: page2 returns books 3-4 → total 4 → offset+len=4
// == total → no next. A third page (?start=4) is empty.
func TestAcquisitionPagination(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	old := pageSize
	pageSize = 2
	defer func() { pageSize = old }()

	hasNext := func(feed atomFeed) bool {
		for _, l := range feed.Links {
			if l.Rel == "next" {
				return true
			}
		}
		return false
	}

	// Page 1.
	rec := opdsGetBasic(h, "/opds/all", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("page1: expected 200, got %d", rec.Code)
	}
	var p1 atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &p1); err != nil {
		t.Fatalf("page1 unmarshal: %v", err)
	}
	if len(p1.Entries) != 2 {
		t.Fatalf("page1 expected 2 entries, got %d", len(p1.Entries))
	}
	if !hasNext(p1) {
		t.Error("page1 expected rel=next (4 total, 2 shown)")
	}

	// Page 2.
	rec = opdsGetBasic(h, "/opds/all?start=2", testEmail, testPassword)
	var p2 atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &p2); err != nil {
		t.Fatalf("page2 unmarshal: %v", err)
	}
	if len(p2.Entries) != 2 {
		t.Fatalf("page2 expected 2 entries, got %d", len(p2.Entries))
	}
	if hasNext(p2) {
		t.Error("page2 should not have rel=next (offset 2 + 2 == 4 total)")
	}
}

// TestAcquisitionUnknownAuthor verifies an unknown author yields an empty feed,
// not an error.
func TestAcquisitionUnknownAuthor(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	rec := opdsGetBasic(h, "/opds/author/"+encodeNameID("Nobody At All"), testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(feed.Entries) != 0 {
		t.Errorf("expected empty feed, got %d entries", len(feed.Entries))
	}
}
