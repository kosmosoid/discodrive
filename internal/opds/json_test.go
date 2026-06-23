package opds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// opdsGetJSON sends a GET with Basic auth and Accept: application/opds+json.
func opdsGetJSON(h *Handler, target, email, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetBasicAuth(email, password)
	req.Header.Set("Accept", mediaTypeOPDS2)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// assertJSONFeed unmarshals the response body into a feed2 and returns it.
func assertJSONFeed(t *testing.T, rec *httptest.ResponseRecorder) feed2 {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, mediaTypeOPDS2) {
		t.Errorf("Content-Type=%q, want contains %q", ct, mediaTypeOPDS2)
	}
	var f feed2
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	return f
}

// TestRootJSON verifies the root nav feed in OPDS 2.0 JSON.
func TestRootJSON(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetJSON(h, "/opds", testEmail, testPassword)
	f := assertJSONFeed(t, rec)

	// metadata.title
	if f.Metadata.Title != "discodrive Library" {
		t.Errorf("metadata.title=%q, want %q", f.Metadata.Title, "discodrive Library")
	}

	// navigation entries: New, By Author, By Series, By Genre, All Books.
	wantNav := map[string]string{
		"New":       "/opds/new",
		"By Author": "/opds/nav/authors",
		"By Series": "/opds/nav/series",
		"By Genre":  "/opds/nav/genres",
		"All Books": "/opds/all",
	}
	got := map[string]string{}
	for _, n := range f.Navigation {
		got[n.Title] = n.Href
	}
	for title, href := range wantNav {
		if gotHref, ok := got[title]; !ok {
			t.Errorf("missing navigation entry %q", title)
		} else if gotHref != href {
			t.Errorf("navigation[%q].href=%q, want %q", title, gotHref, href)
		}
	}

	// links: rel=self (type=application/opds+json) + rel=search (templated).
	var hasSelf, hasSearch bool
	for _, l := range f.Links {
		if l.Rel == "self" && l.Href == "/opds" && l.Type == mediaTypeOPDS2 {
			hasSelf = true
		}
		if l.Rel == "search" && l.Templated {
			hasSearch = true
			if !strings.Contains(l.Href, "{?query}") {
				t.Errorf("search link href=%q, want template containing {?query}", l.Href)
			}
		}
	}
	if !hasSelf {
		t.Errorf("missing self link (href=/opds, type=%s)", mediaTypeOPDS2)
	}
	if !hasSearch {
		t.Error("missing templated search link with rel=search")
	}
}

// TestRootAtomDefault verifies the root returns Atom when no JSON Accept is set.
func TestRootAtomDefault(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetBasic(h, "/opds", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "atom+xml") {
		t.Errorf("Content-Type=%q, want atom+xml (Atom default)", ct)
	}
}

// TestNavAuthorsJSON verifies the authors nav feed in OPDS 2.0 JSON.
func TestNavAuthorsJSON(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetJSON(h, "/opds/nav/authors", testEmail, testPassword)
	f := assertJSONFeed(t, rec)

	if f.Metadata.Title != "Authors" {
		t.Errorf("metadata.title=%q, want %q", f.Metadata.Title, "Authors")
	}
	if len(f.Navigation) < 2 {
		t.Fatalf("expected >=2 navigation entries, got %d", len(f.Navigation))
	}
	names := map[string]bool{"Ada Lovelace": false, "Isaac Asimov": false}
	for _, n := range f.Navigation {
		if _, ok := names[n.Title]; ok {
			names[n.Title] = true
		}
		if !strings.Contains(n.Href, "/opds/author/") {
			t.Errorf("navigation entry %q href=%q, want contains /opds/author/", n.Title, n.Href)
		}
		if n.Type != mediaTypeOPDS2 {
			t.Errorf("navigation entry %q type=%q, want %q", n.Title, n.Type, mediaTypeOPDS2)
		}
	}
	for name, found := range names {
		if !found {
			t.Errorf("expected author %q in navigation, not found", name)
		}
	}
}

// TestNavAuthorsAtomDefault verifies authors feed stays Atom without JSON Accept.
func TestNavAuthorsAtomDefault(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetBasic(h, "/opds/nav/authors", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "atom+xml") {
		t.Errorf("Content-Type=%q, want atom+xml", ct)
	}
}

// TestNavSeriesJSON verifies the series nav feed in OPDS 2.0 JSON.
func TestNavSeriesJSON(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetJSON(h, "/opds/nav/series", testEmail, testPassword)
	f := assertJSONFeed(t, rec)

	if f.Metadata.Title != "Series" {
		t.Errorf("metadata.title=%q, want %q", f.Metadata.Title, "Series")
	}
	if len(f.Navigation) < 2 {
		t.Fatalf("expected >=2 navigation entries, got %d", len(f.Navigation))
	}
	names := map[string]bool{"Foundation": false, "History": false}
	for _, n := range f.Navigation {
		if _, ok := names[n.Title]; ok {
			names[n.Title] = true
		}
		if !strings.Contains(n.Href, "/opds/series/") {
			t.Errorf("navigation entry %q href=%q, want /opds/series/", n.Title, n.Href)
		}
	}
	for name, found := range names {
		if !found {
			t.Errorf("expected series %q in navigation, not found", name)
		}
	}
}

// TestNavSeriesAtomDefault verifies series feed stays Atom without JSON Accept.
func TestNavSeriesAtomDefault(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetBasic(h, "/opds/nav/series", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "atom+xml") {
		t.Errorf("Content-Type=%q, want atom+xml", ct)
	}
}

// TestNavGenresJSON verifies the genres nav feed in OPDS 2.0 JSON.
func TestNavGenresJSON(t *testing.T) {
	h, ctx := setupOPDS(t)
	user, err := h.q.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	seedBooks(t, h, user.ID)

	rec := opdsGetJSON(h, "/opds/nav/genres", testEmail, testPassword)
	f := assertJSONFeed(t, rec)

	if f.Metadata.Title != "Genres" {
		t.Errorf("metadata.title=%q, want %q", f.Metadata.Title, "Genres")
	}
	if len(f.Navigation) < 2 {
		t.Fatalf("expected >=2 navigation entries, got %d", len(f.Navigation))
	}
	names := map[string]bool{"Science": false, "SciFi": false}
	for _, n := range f.Navigation {
		if _, ok := names[n.Title]; ok {
			names[n.Title] = true
		}
		if !strings.Contains(n.Href, "/opds/genre/") {
			t.Errorf("navigation entry %q href=%q, want /opds/genre/", n.Title, n.Href)
		}
	}
	for name, found := range names {
		if !found {
			t.Errorf("expected genre %q in navigation, not found", name)
		}
	}
}

// TestNavGenresAtomDefault verifies genres feed stays Atom without JSON Accept.
func TestNavGenresAtomDefault(t *testing.T) {
	h, _ := setupOPDS(t)

	rec := opdsGetBasic(h, "/opds/nav/genres", testEmail, testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "atom+xml") {
		t.Errorf("Content-Type=%q, want atom+xml", ct)
	}
}
