package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"discodrive/internal/auth"
	"discodrive/internal/bookmarks"
	"discodrive/internal/db"
	"discodrive/internal/storage"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func timeNowPlusHour() time.Time { return time.Now().Add(time.Hour) }

// bookmarkHarness wires a Server with a real DB for bookmark tests.
type bookmarkHarness struct {
	s    *Server
	svc  *auth.Service
	pool *pgxpool.Pool
	q    *db.Queries
	tok  string
	uid  pgtype.UUID
}

func newBookmarkHarness(t *testing.T) *bookmarkHarness {
	t.Helper()
	ctx := context.Background()
	pool, q, svc := bootstrapPairingDB(t)
	tok, _, err := svc.Register(ctx, "bm@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	root := t.TempDir()
	bmSvc := bookmarks.NewService(pool, q, storage.NewLocalDisk(root))
	bmSvc.Client = &http.Client{}
	bmSvc.Validate = func(string) error { return nil }
	s := &Server{auth: svc, q: q, bookmarks: bmSvc, storageRoot: root}
	uid, _ := db.ParseUUID(mustUserID(t, svc, tok))
	return &bookmarkHarness{s: s, svc: svc, pool: pool, q: q, tok: tok, uid: uid}
}

func (h *bookmarkHarness) do(t *testing.T, method, path string, body any, pathID string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var handler http.HandlerFunc
	switch {
	case method == http.MethodGet && path == "/me/bookmarks":
		handler = h.s.handleBookmarksList
	case method == http.MethodGet: // changes
		handler = h.s.handleBookmarksChanges
	case method == http.MethodPost && path == "/me/bookmarks/bulk":
		handler = h.s.handleBookmarksBulk
	case method == http.MethodPost:
		handler = h.s.handleBookmarkCreate
	case method == http.MethodPatch:
		handler = h.s.handleBookmarkUpdate
	case method == http.MethodDelete:
		handler = h.s.handleBookmarkDelete
	}
	rec, m := doJSON(h.svc.Middleware(handler), method, path, h.tok, body, pathID)
	return rec, m
}

// doJSON runs a JSON request through a middleware-wrapped handler.
func doJSON(handler http.Handler, method, path, bearer string, body any, pathID string) (*httptest.ResponseRecorder, map[string]any) {
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytesReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if pathID != "" {
		req.SetPathValue("id", pathID)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

func (h *bookmarkHarness) changes(t *testing.T, since string) (map[string]any, []map[string]any) {
	t.Helper()
	rec, m := h.do(t, http.MethodGet, "/me/bookmarks/changes?since="+since, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("changes since=%s: %d %s", since, rec.Code, rec.Body.String())
	}
	var items []map[string]any
	for _, it := range m["items"].([]any) {
		items = append(items, it.(map[string]any))
	}
	return m, items
}

func TestBookmarksCRUDAndChanges(t *testing.T) {
	h := newBookmarkHarness(t)

	// Create a folder, then a bookmark inside it (client-generated ids).
	folderID := uuid.NewString()
	rec, m := h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{
		"id": folderID, "is_folder": true, "title": "Работа",
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("create folder: %d %s", rec.Code, rec.Body.String())
	}
	if m["id"] != folderID {
		t.Fatalf("folder id = %v, want the client-generated one", m["id"])
	}
	bmID := uuid.NewString()
	if rec, _ := h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{
		"id": bmID, "parent_id": folderID, "title": "Го", "url": "https://go.dev", "position": 1,
	}, ""); rec.Code != http.StatusOK {
		t.Fatalf("create bookmark: %d %s", rec.Code, rec.Body.String())
	}

	// Idempotent retry: same id → 200 with the same row, no duplicate.
	if rec, m2 := h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{
		"id": bmID, "parent_id": folderID, "title": "Го", "url": "https://go.dev",
	}, ""); rec.Code != http.StatusOK || m2["id"] != bmID {
		t.Fatalf("idempotent retry: %d %v", rec.Code, m2)
	}
	// A url-less bookmark is invalid; a folder with a parent that is not a
	// folder is invalid.
	if rec, _ := h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{"title": "x"}, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bookmark without url: expected 400, got %d", rec.Code)
	}
	if rec, _ := h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{
		"parent_id": bmID, "title": "x", "url": "https://x",
	}, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("parent-not-a-folder: expected 400, got %d", rec.Code)
	}

	// Someone else's id → 409.
	tok2, _, err := h.svc.Register(context.Background(), "bm2@x.test", "password12")
	if err != nil {
		t.Fatalf("register2: %v", err)
	}
	if rec, _ := doJSON(h.svc.Middleware(http.HandlerFunc(h.s.handleBookmarkCreate)), http.MethodPost, "/me/bookmarks",
		tok2, map[string]any{"id": bmID, "title": "steal", "url": "https://x"}, ""); rec.Code != http.StatusConflict {
		t.Fatalf("foreign id: expected 409, got %d", rec.Code)
	}

	// List contains both, tree-shaped via parent_id.
	recL, _ := h.do(t, http.MethodGet, "/me/bookmarks", nil, "")
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("list: expected 2 nodes, got %d: %s", len(list), recL.Body.String())
	}

	// Changes feed from zero: both rows, cursor advances with edits.
	feed, items := h.changes(t, "0")
	if len(items) != 2 || feed["full_resync"] == true {
		t.Fatalf("changes: %v", feed)
	}
	cursor := feed["cursor"].(float64)

	// Rename via PATCH → exactly one new change after the old cursor.
	if rec, _ := h.do(t, http.MethodPatch, "/me/bookmarks/"+bmID, map[string]any{"title": "Go"}, bmID); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	_, items2 := h.changes(t, formatFloat(cursor))
	if len(items2) != 1 || items2[0]["id"] != bmID || items2[0]["title"] != "Go" {
		t.Fatalf("changes after rename: %v", items2)
	}

	// Delete the folder → the whole subtree is tombstoned with one seq.
	if rec, _ := h.do(t, http.MethodDelete, "/me/bookmarks/"+folderID, nil, folderID); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	_, itemsDel := h.changes(t, formatFloat(cursor))
	deleted := 0
	for _, it := range itemsDel {
		if it["deleted"] == true {
			deleted++
		}
	}
	if deleted != 2 {
		t.Fatalf("expected 2 tombstones in the feed, got %d: %v", deleted, itemsDel)
	}
	recL2, _ := h.do(t, http.MethodGet, "/me/bookmarks", nil, "")
	var list2 []map[string]any
	_ = json.Unmarshal(recL2.Body.Bytes(), &list2)
	if len(list2) != 0 {
		t.Fatalf("list after delete must be empty, got %s", recL2.Body.String())
	}
	// Double delete → 404.
	if rec, _ := h.do(t, http.MethodDelete, "/me/bookmarks/"+folderID, nil, folderID); rec.Code != http.StatusNotFound {
		t.Fatalf("double delete: expected 404, got %d", rec.Code)
	}
}

func TestBookmarksMoveCycle(t *testing.T) {
	h := newBookmarkHarness(t)
	a, b := uuid.NewString(), uuid.NewString()
	h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{"id": a, "is_folder": true, "title": "A"}, "")
	h.do(t, http.MethodPost, "/me/bookmarks", map[string]any{"id": b, "parent_id": a, "is_folder": true, "title": "B"}, "")

	// Moving A under its own descendant B must fail.
	if rec, _ := h.do(t, http.MethodPatch, "/me/bookmarks/"+a, map[string]any{"parent_id": b}, a); rec.Code != http.StatusConflict {
		t.Fatalf("cycle move: expected 409, got %d", rec.Code)
	}
	// Moving B to the root works (parent_id: "").
	if rec, m := h.do(t, http.MethodPatch, "/me/bookmarks/"+b, map[string]any{"parent_id": ""}, b); rec.Code != http.StatusOK || m["parent_id"] != nil {
		t.Fatalf("move to root: %d %v", rec.Code, m)
	}
}

func TestBookmarksBulkAndFullResync(t *testing.T) {
	h := newBookmarkHarness(t)
	root, child, leaf := uuid.NewString(), uuid.NewString(), uuid.NewString()
	items := []map[string]any{
		{"id": root, "is_folder": true, "title": "Панель закладок"},
		{"id": child, "parent_id": root, "is_folder": true, "title": "Dev"},
		{"id": leaf, "parent_id": child, "title": "Go", "url": "https://go.dev", "position": 3},
	}
	rec, m := h.do(t, http.MethodPost, "/me/bookmarks/bulk", map[string]any{"items": items}, "")
	if rec.Code != http.StatusOK || m["count"].(float64) != 3 {
		t.Fatalf("bulk: %d %v", rec.Code, m)
	}
	cursor := m["cursor"].(float64)

	// Re-import is idempotent (LWW upsert), no duplicates.
	if rec, _ := h.do(t, http.MethodPost, "/me/bookmarks/bulk", map[string]any{"items": items}, ""); rec.Code != http.StatusOK {
		t.Fatalf("re-bulk: %d", rec.Code)
	}
	recL, _ := h.do(t, http.MethodGet, "/me/bookmarks", nil, "")
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 3 {
		t.Fatalf("after re-bulk: expected 3 nodes, got %d", len(list))
	}

	// full_resync: delete a node, GC with zero retention, then ask with a
	// pre-GC cursor — the server must demand a full resync.
	if rec, _ := h.do(t, http.MethodDelete, "/me/bookmarks/"+leaf, nil, leaf); rec.Code != http.StatusNoContent {
		t.Fatalf("delete leaf: %d", rec.Code)
	}
	ctx := context.Background()
	rows, err := h.q.GCBrowserBookmarkTombstones(ctx, pgtype.Timestamptz{Time: timeNowPlusHour(), Valid: true})
	if err != nil || len(rows) != 1 {
		t.Fatalf("gc: %v rows=%d", err, len(rows))
	}
	if err := h.q.BumpBookmarkGCSeq(ctx, db.BumpBookmarkGCSeqParams{ID: h.uid, BookmarkGcSeq: rows[0].Seq}); err != nil {
		t.Fatalf("bump gc seq: %v", err)
	}
	feed, _ := h.changes(t, formatFloat(cursor))
	if feed["full_resync"] != true {
		t.Fatalf("expected full_resync for a pre-GC cursor, got %v", feed)
	}
	// A fresh client (since=0) keeps working without full_resync... since=0 is
	// also below the watermark, but 0 means "give me everything" — the initial
	// pull IS the full state, so full_resync must not trigger.
	feed0, items0 := h.changes(t, "0")
	if feed0["full_resync"] == true || len(items0) != 2 {
		t.Fatalf("since=0 pull: %v (%d items)", feed0, len(items0))
	}
}
