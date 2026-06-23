package opds

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// parsedOpenSearch is a minimal struct for parsing an OpenSearch description document in tests.
type parsedOpenSearch struct {
	XMLName     xml.Name           `xml:"OpenSearchDescription"`
	ShortName   string             `xml:"ShortName"`
	Description string             `xml:"Description"`
	Urls        []parsedSearchURL  `xml:"Url"`
}

type parsedSearchURL struct {
	Type     string `xml:"type,attr"`
	Template string `xml:"template,attr"`
}

// TestSearchDescription verifies GET /opds/search.xml returns a valid OpenSearch
// description document with Url templates for both Atom and OPDS 2.0 JSON.
func TestSearchDescription(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetBasic(h, "/opds/search.xml", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/opensearchdescription+xml") {
		t.Errorf("Content-Type=%q, want application/opensearchdescription+xml", ct)
	}

	var doc parsedOpenSearch
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("xml.Unmarshal: %v\nbody: %s", err, rec.Body.String())
	}

	if doc.ShortName == "" {
		t.Error("ShortName is empty")
	}
	if doc.Description == "" {
		t.Error("Description is empty")
	}

	// Both Atom acquisition and OPDS 2.0 JSON Url elements must be present.
	var hasAtom, hasJSON bool
	for _, u := range doc.Urls {
		if !strings.Contains(u.Template, "/opds/search?q={searchTerms}") {
			t.Errorf("Url template=%q, want to contain /opds/search?q={searchTerms}", u.Template)
		}
		if strings.Contains(u.Type, "application/atom+xml") {
			hasAtom = true
		}
		if strings.Contains(u.Type, "application/opds+json") {
			hasJSON = true
		}
	}
	if !hasAtom {
		t.Error("missing Url with atom+xml type")
	}
	if !hasJSON {
		t.Error("missing Url with application/opds+json type")
	}
}

// TestSearchByTitle verifies GET /opds/search?q=<title substring> returns an
// acquisition feed containing the matching book and no unrelated books.
func TestSearchByTitle(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	rec := opdsGetBasic(h, "/opds/search?q=Book+One", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, mediaTypeAcquisition) {
		t.Errorf("Content-Type=%q, want %q", ct, mediaTypeAcquisition)
	}

	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	if findEntry(feed, "Book One") == nil {
		t.Error("expected Book One in results")
	}
	if findEntry(feed, "Book Two") != nil {
		t.Error("unexpected Book Two in results")
	}
	if findEntry(feed, "Secret Book") != nil {
		t.Error("user2 book leaked into search results")
	}
}

// TestSearchByAuthor verifies ?q=<author name substring> finds books by that author.
func TestSearchByAuthor(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	// "Asimov" matches only Book Two (authored by Isaac Asimov).
	rec := opdsGetBasic(h, "/opds/search?q=Asimov", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if findEntry(feed, "Book Two") == nil {
		t.Error("expected Book Two (Asimov) in results")
	}
	if findEntry(feed, "Book One") != nil {
		t.Error("unexpected Book One in Asimov search")
	}
	if findEntry(feed, "Secret Book") != nil {
		t.Error("user2 book leaked into author search results")
	}
}

// TestSearchJSON verifies Accept: application/opds+json on a search returns
// OPDS 2.0 JSON publications.
func TestSearchJSON(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	rec := opdsGetJSON(h, "/opds/search?q=Foundation", testEmail, testPassword)
	f := assertJSONFeed(t, rec)

	var found bool
	for _, p := range f.Publications {
		if p.Metadata.Title == "Book Two" {
			found = true
		}
		if p.Metadata.Title == "Secret Book" {
			t.Error("user2 book leaked into JSON search results")
		}
	}
	if !found {
		t.Error("expected Book Two (Foundation series) in JSON search results")
	}
}

// TestSearchEmptyQuery verifies GET /opds/search (no q) returns 200 with an
// empty acquisition feed — not a 400 or panic.
func TestSearchEmptyQuery(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	rec := opdsGetBasic(h, "/opds/search", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal empty feed: %v\nbody: %s", err, rec.Body.String())
	}
	if len(feed.Entries) != 0 {
		t.Errorf("empty query should return 0 entries, got %d", len(feed.Entries))
	}
}

// TestSearchScopeIsolation verifies that a query matching user2's book title
// returns an empty feed when authenticated as user1.
func TestSearchScopeIsolation(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h) // seeds "Secret Book" owned by user2, not shared

	rec := opdsGetBasic(h, "/opds/search?q=Secret", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var feed atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	if len(feed.Entries) != 0 {
		t.Errorf("expected 0 entries (user2 book must not appear), got %d", len(feed.Entries))
	}
}

// TestSearchQueryAliases verifies that the ?query= and ?searchTerms= parameters
// also work as aliases for ?q=.
func TestSearchQueryAliases(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h)

	for _, param := range []string{"query", "searchTerms"} {
		t.Run(param, func(t *testing.T) {
			target := "/opds/search?" + param + "=Book+One"
			rec := opdsGetBasic(h, target, testEmail, testPassword)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
			}
			var feed atomFeed
			if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if findEntry(feed, "Book One") == nil {
				t.Errorf("param %q: expected Book One in results", param)
			}
		})
	}
}

// TestSearchEmptyQueryJSON verifies empty query also returns empty JSON feed.
func TestSearchEmptyQueryJSON(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetJSON(h, "/opds/search", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var f feed2
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	if len(f.Publications) != 0 {
		t.Errorf("empty JSON query should return 0 publications, got %d", len(f.Publications))
	}
}

// TestSearchPagination verifies that a search result spanning multiple pages
// emits a rel=next link whose href has exactly one '?' (i.e. uses '&' as the
// separator between q= and start=), and that page 2 returns the remaining
// matches without a rel=next.
func TestSearchPagination(t *testing.T) {
	h, _ := setupOPDS(t)
	seedAcqBooks(t, h) // seeds 4 books for user1: Book One/Two/Three/Four

	old := pageSize
	pageSize = 3
	defer func() { pageSize = old }()

	// Query "Book" matches all four user1 books. With pageSize=3:
	// page 1 returns 3 (full page, has next), page 2 returns 1 (partial, no next).
	rec := opdsGetBasic(h, "/opds/search?q=Book", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("page1: expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var p1 atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &p1); err != nil {
		t.Fatalf("page1 unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	if len(p1.Entries) != 3 {
		t.Fatalf("page1: expected 3 entries, got %d", len(p1.Entries))
	}

	// Find the rel=next link and validate its URL structure.
	var nextHref string
	for _, l := range p1.Links {
		if l.Rel == "next" {
			nextHref = l.Href
			break
		}
	}
	if nextHref == "" {
		t.Fatal("page1: expected rel=next link but none found")
	}
	if strings.Count(nextHref, "?") != 1 {
		t.Errorf("page1 next-link has %d '?' characters, want exactly 1: %q", strings.Count(nextHref, "?"), nextHref)
	}
	if !strings.Contains(nextHref, "q=") {
		t.Errorf("page1 next-link missing q= param: %q", nextHref)
	}
	if !strings.Contains(nextHref, "&start=") {
		t.Errorf("page1 next-link missing &start= separator: %q", nextHref)
	}

	// Page 2: request using the offset from the next-link.
	rec = opdsGetBasic(h, "/opds/search?q=Book&start="+strconv.Itoa(pageSize), testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("page2: expected 200, got %d", rec.Code)
	}

	var p2 atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &p2); err != nil {
		t.Fatalf("page2 unmarshal: %v", err)
	}
	// Page 2 is partial (1 of 4 matches at offset 3), so no rel=next.
	if len(p2.Entries) != 1 {
		t.Fatalf("page2: expected 1 entry (partial page), got %d", len(p2.Entries))
	}
	for _, l := range p2.Links {
		if l.Rel == "next" {
			t.Errorf("page2 should have no rel=next (partial page returned), but got href=%q", l.Href)
		}
	}
}
