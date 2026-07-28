package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discodrive/internal/auth"
	"discodrive/internal/db"
	"discodrive/internal/saved"
	"discodrive/internal/storage"
)

// TestSavedCRUD exercises POST/GET/retry/DELETE on /me/saved end-to-end
// against a real Postgres and a local httptest download server. Downloads are
// an internal queue: pollable by id, never present in listings.
func TestSavedCRUD(t *testing.T) {
	ctx := context.Background()
	pool, q, svc := bootstrapPairingDB(t)
	_ = pool

	tok, _, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	root := t.TempDir()
	savedSvc := saved.NewService(q, storage.NewLocalDisk(root), 0)
	savedSvc.Client = &http.Client{}
	savedSvc.Validate = func(string) error { return nil }
	s := &Server{auth: svc, q: q, saved: savedSvc, storageRoot: root}

	createH := svc.Middleware(http.HandlerFunc(s.handleSavedCreate))
	listH := svc.Middleware(http.HandlerFunc(s.handleSavedList))
	getH := svc.Middleware(http.HandlerFunc(s.handleSavedGet))

	dl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer dl.Close()

	// Create a download.
	rec, m := doPost(createH, "/me/saved", tok, map[string]any{"url": dl.URL + "/file.bin", "kind": "download"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatalf("create: no id in %v", m)
	}

	// Validation: unknown kind, empty and overlong URLs are 400s.
	if rec, _ := doPost(createH, "/me/saved", tok, map[string]any{"url": "https://x/y", "kind": "bookmark"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bookmark kind must be rejected now: expected 400, got %d", rec.Code)
	}
	if rec, _ := doPost(createH, "/me/saved", tok, map[string]any{"url": "", "kind": "article"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty url: expected 400, got %d", rec.Code)
	}
	if rec, _ := doPost(createH, "/me/saved", tok, map[string]any{"url": "https://x/" + strings.Repeat("a", 2100), "kind": "article"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("overlong url: expected 400, got %d", rec.Code)
	}

	// Re-saving the same URL+kind must return the same item, not a duplicate.
	if _, m2 := doPost(createH, "/me/saved", tok, map[string]any{"url": dl.URL + "/file.bin", "kind": "download"}); m2["id"] != id {
		t.Fatalf("re-save returned a different id: %v vs %s", m2["id"], id)
	}

	waitItemStatus(t, getH, tok, id, "done")

	// Listings never contain downloads; kind=download filter is rejected.
	if recL, _ := doGet(listH, "/me/saved", tok); strings.Contains(recL.Body.String(), id) {
		t.Fatalf("downloads must not appear in listings: %s", recL.Body.String())
	}
	if recL, _ := doGet(listH, "/me/saved?kind=download", tok); recL.Code != http.StatusBadRequest {
		t.Fatalf("kind=download filter: expected 400, got %d", recL.Code)
	}
	if recL, _ := doGet(listH, "/me/saved?kind=article", tok); recL.Code != http.StatusOK {
		t.Fatalf("kind=article filter: expected 200, got %d", recL.Code)
	}

	// A second user must not see the item by id.
	tok2, _, err := svc.Register(ctx, "other@x.test", "password12")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	if rec6 := doPathReq(getH, http.MethodGet, "/me/saved/"+id, tok2, id); rec6.Code != http.StatusNotFound {
		t.Fatalf("foreign get: expected 404, got %d", rec6.Code)
	}

	// Retry: done → pending → done again. Retry by the wrong user is a 409.
	retryH := svc.Middleware(http.HandlerFunc(s.handleSavedRetry))
	if recBad := doPathReq(retryH, http.MethodPost, "/me/saved/"+id+"/retry", tok2, id); recBad.Code != http.StatusConflict {
		t.Fatalf("foreign retry: expected 409, got %d", recBad.Code)
	}
	if recRetry := doPathReq(retryH, http.MethodPost, "/me/saved/"+id+"/retry", tok, id); recRetry.Code != http.StatusOK {
		t.Fatalf("retry: expected 200, got %d: %s", recRetry.Code, recRetry.Body.String())
	}
	waitItemStatus(t, getH, tok, id, "done")

	// DELETE removes the record but keeps the downloaded file (it lives in the
	// user's tree and is managed by the Files section).
	uid, _ := db.ParseUUID(mustUserID(t, svc, tok))
	itemUUID, _ := db.ParseUUID(id)
	before, err := q.GetSavedItemForUser(ctx, db.GetSavedItemForUserParams{ID: itemUUID, UserID: uid})
	if err != nil {
		t.Fatalf("get before delete: %v", err)
	}
	if !strings.Contains(before.ContentPath.String, "/Downloads/") {
		t.Fatalf("content_path = %q, want the Downloads folder", before.ContentPath.String)
	}
	deleteH := svc.Middleware(http.HandlerFunc(s.handleSavedDelete))
	if recDel := doPathReq(deleteH, http.MethodDelete, "/me/saved/"+id, tok, id); recDel.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", recDel.Code)
	}
	if _, err := os.Stat(filepath.Join(root, before.ContentPath.String)); err != nil {
		t.Fatalf("downloaded file must survive record deletion: %v", err)
	}
	if recDel2 := doPathReq(deleteH, http.MethodDelete, "/me/saved/"+id, tok, id); recDel2.Code != http.StatusNotFound {
		t.Fatalf("double delete: expected 404, got %d", recDel2.Code)
	}
}

// TestSavedContent covers the article markdown endpoint, incl. tenant isolation.
func TestSavedContent(t *testing.T) {
	ctx := context.Background()
	_, q, svc := bootstrapPairingDB(t)

	tok, _, err := svc.Register(ctx, "art@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	root := t.TempDir()
	savedSvc := saved.NewService(q, storage.NewLocalDisk(root), 0)
	savedSvc.Client = &http.Client{}
	savedSvc.Validate = func(string) error { return nil }
	s := &Server{auth: svc, q: q, saved: savedSvc, storageRoot: root}

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Post</title></head><body><article><h1>Post</h1>` +
			`<p>Enough meaningful text for readability to accept this page as an article. ` +
			`More words, and even more words, to be safely above any length threshold.</p>` +
			`<p>Second paragraph with plenty of additional supporting content in it.</p>` +
			`</article></body></html>`))
	}))
	defer site.Close()

	createH := svc.Middleware(http.HandlerFunc(s.handleSavedCreate))
	getH := svc.Middleware(http.HandlerFunc(s.handleSavedGet))
	contentH := svc.Middleware(http.HandlerFunc(s.handleSavedContent))

	rec, m := doPost(createH, "/me/saved", tok, map[string]any{"url": site.URL + "/post", "kind": "article"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id, _ := m["id"].(string)
	waitItemStatus(t, getH, tok, id, "done")

	recC := doPathReq(contentH, http.MethodGet, "/me/saved/"+id+"/content", tok, id)
	if recC.Code != http.StatusOK {
		t.Fatalf("content: %d %s", recC.Code, recC.Body.String())
	}
	if ct := recC.Header().Get("Content-Type"); !strings.Contains(ct, "text/markdown") {
		t.Fatalf("content type = %q", ct)
	}
	if body := recC.Body.String(); !strings.HasPrefix(body, "---\n") || !strings.Contains(body, "Enough meaningful text") {
		t.Fatalf("unexpected content:\n%.300s", body)
	}

	// Another user gets a 404 for someone else's article.
	tok2, _, err := svc.Register(ctx, "art2@x.test", "password12")
	if err != nil {
		t.Fatalf("register2: %v", err)
	}
	if rec2 := doPathReq(contentH, http.MethodGet, "/me/saved/"+id+"/content", tok2, id); rec2.Code != http.StatusNotFound {
		t.Fatalf("foreign content: expected 404, got %d", rec2.Code)
	}

	// Content of a download is a 404 (only articles store readable content).
	recB, mB := doPost(createH, "/me/saved", tok, map[string]any{"url": site.URL + "/f.html", "kind": "download"})
	if recB.Code != http.StatusOK {
		t.Fatalf("create download: %d", recB.Code)
	}
	dlID, _ := mB["id"].(string)
	waitItemStatus(t, getH, tok, dlID, "done")
	if recDL := doPathReq(contentH, http.MethodGet, "/me/saved/"+dlID+"/content", tok, dlID); recDL.Code != http.StatusNotFound {
		t.Fatalf("download content: expected 404, got %d", recDL.Code)
	}
}

// doPathReq performs a request against a {id}-routed handler with the path value set.
func doPathReq(h http.Handler, method, path, bearer, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// waitItemStatus polls GET /me/saved/{id} until the item reports the wanted status.
func waitItemStatus(t *testing.T, getH http.Handler, tok, id, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		rec := doPathReq(getH, http.MethodGet, "/me/saved/"+id, tok, id)
		if strings.Contains(rec.Body.String(), `"status":"`+want+`"`) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("item %s never reached %q: %s", id, want, rec.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// mustUserID resolves the user id behind a session token via the auth middleware.
func mustUserID(t *testing.T, svc interface {
	Middleware(http.Handler) http.Handler
}, tok string) string {
	t.Helper()
	var uid string
	h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid = auth.UserID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if uid == "" {
		t.Fatal("could not resolve user id from token")
	}
	return uid
}
