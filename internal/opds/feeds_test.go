package opds

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// seedBooks inserts test books with authors/series/genres for navigation feed tests.
func seedBooks(t *testing.T, h *Handler, userID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()

	// Create a folder node to hang books from.
	node, err := h.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   userID,
		ParentID: pgtype.UUID{Valid: false},
		Name:     "ebooks",
		IsDir:    true,
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// Book 1: author=Ada Lovelace, series=History, genre=Science.
	b1, err := h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      userID,
		NodeID:      node.ID,
		Title:       "Book One",
		SortTitle:   "book one",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Series:      pgtype.Text{String: "History", Valid: true},
		SeriesIndex: pgtype.Float4{Float32: 1, Valid: true},
		Size:        pgtype.Int8{Int64: 1000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook b1: %v", err)
	}
	if err := h.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID:   b1.ID,
		Name:     "Ada Lovelace",
		SortName: "lovelace, ada",
	}); err != nil {
		t.Fatalf("InsertBookAuthor b1: %v", err)
	}
	if err := h.q.InsertBookTag(ctx, db.InsertBookTagParams{
		BookID: b1.ID,
		Tag:    "Science",
	}); err != nil {
		t.Fatalf("InsertBookTag b1: %v", err)
	}

	// Create a second folder node for the second book to satisfy the UNIQUE node_id constraint.
	node2, err := h.q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   userID,
		ParentID: pgtype.UUID{Valid: false},
		Name:     "ebooks2",
		IsDir:    true,
	})
	if err != nil {
		t.Fatalf("CreateNode node2: %v", err)
	}

	// Book 2: author=Isaac Asimov, series=Foundation, genre=SciFi.
	b2, err := h.q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      userID,
		NodeID:      node2.ID,
		Title:       "Book Two",
		SortTitle:   "book two",
		Format:      "epub",
		ContentType: "application/epub+zip",
		Series:      pgtype.Text{String: "Foundation", Valid: true},
		SeriesIndex: pgtype.Float4{Float32: 1, Valid: true},
		Size:        pgtype.Int8{Int64: 2000, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook b2: %v", err)
	}
	if err := h.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID:   b2.ID,
		Name:     "Isaac Asimov",
		SortName: "asimov, isaac",
	}); err != nil {
		t.Fatalf("InsertBookAuthor b2: %v", err)
	}
	if err := h.q.InsertBookTag(ctx, db.InsertBookTagParams{
		BookID: b2.ID,
		Tag:    "SciFi",
	}); err != nil {
		t.Fatalf("InsertBookTag b2: %v", err)
	}
}

func TestRootNavFeed(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetBasic(h, "/opds", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, mediaTypeNavigation) {
		t.Errorf("Content-Type=%q, want contains %q", ct, mediaTypeNavigation)
	}

	// Verify xmlns attrs appear in the raw output.
	body := rec.Body.Bytes()
	if !strings.Contains(string(body), `xmlns="http://www.w3.org/2005/Atom"`) {
		t.Error("missing Atom xmlns in output")
	}
	if !strings.Contains(string(body), `xmlns:opds="http://opds-spec.org/2010/catalog"`) {
		t.Error("missing OPDS xmlns:opds in output")
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		t.Fatalf("unmarshal feed: %v\nbody: %s", err, body)
	}

	// Check required feed fields.
	if feed.ID == "" {
		t.Error("feed.id is empty")
	}
	if feed.Title == "" {
		t.Error("feed.title is empty")
	}
	if feed.Updated == "" {
		t.Error("feed.updated is empty")
	}

	// Check expected entries.
	wantTitles := []string{"New", "By Author", "By Series", "By Genre", "All Books"}
	entryTitles := make(map[string]bool)
	for _, e := range feed.Entries {
		entryTitles[e.Title] = true
	}
	for _, want := range wantTitles {
		if !entryTitles[want] {
			t.Errorf("missing entry %q; got titles: %v", want, entryTitles)
		}
	}

	// Check rel=search link.
	var hasSearch bool
	for _, l := range feed.Links {
		if l.Rel == "search" {
			hasSearch = true
		}
	}
	if !hasSearch {
		t.Error("missing rel=search link")
	}
}

func TestNavAuthorsFeed(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetBasic(h, "/opds/nav/authors", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/atom+xml") {
		t.Errorf("Content-Type=%q, want atom+xml", ct)
	}

	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(feed.Entries) < 2 {
		t.Fatalf("expected >=2 author entries, got %d", len(feed.Entries))
	}

	// Verify each entry has a subsection link with an href containing /opds/author/.
	authorNames := map[string]bool{"Ada Lovelace": false, "Isaac Asimov": false}
	for _, e := range feed.Entries {
		if _, ok := authorNames[e.Title]; ok {
			authorNames[e.Title] = true
		}
		var hasSubsection bool
		for _, l := range e.Links {
			if l.Rel == "subsection" && strings.Contains(l.Href, "/opds/author/") {
				hasSubsection = true
			}
		}
		if !hasSubsection {
			t.Errorf("entry %q missing subsection link to /opds/author/", e.Title)
		}
	}
	for name, found := range authorNames {
		if !found {
			t.Errorf("expected author entry %q not found in feed", name)
		}
	}
}

func TestNavSeriesFeed(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetBasic(h, "/opds/nav/series", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(feed.Entries) < 2 {
		t.Fatalf("expected >=2 series entries, got %d", len(feed.Entries))
	}

	seriesNames := map[string]bool{"Foundation": false, "History": false}
	for _, e := range feed.Entries {
		if _, ok := seriesNames[e.Title]; ok {
			seriesNames[e.Title] = true
		}
		var hasSubsection bool
		for _, l := range e.Links {
			if l.Rel == "subsection" && strings.Contains(l.Href, "/opds/series/") {
				hasSubsection = true
			}
		}
		if !hasSubsection {
			t.Errorf("entry %q missing subsection link to /opds/series/", e.Title)
		}
	}
	for name, found := range seriesNames {
		if !found {
			t.Errorf("expected series entry %q not found in feed", name)
		}
	}
}

func TestNavGenresFeed(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetBasic(h, "/opds/nav/genres", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(feed.Entries) < 2 {
		t.Fatalf("expected >=2 genre entries, got %d", len(feed.Entries))
	}

	genreNames := map[string]bool{"Science": false, "SciFi": false}
	for _, e := range feed.Entries {
		if _, ok := genreNames[e.Title]; ok {
			genreNames[e.Title] = true
		}
		var hasSubsection bool
		for _, l := range e.Links {
			if l.Rel == "subsection" && strings.Contains(l.Href, "/opds/genre/") {
				hasSubsection = true
			}
		}
		if !hasSubsection {
			t.Errorf("entry %q missing subsection link to /opds/genre/", e.Title)
		}
	}
	for name, found := range genreNames {
		if !found {
			t.Errorf("expected genre entry %q not found in feed", name)
		}
	}
}
