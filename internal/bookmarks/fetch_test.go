package bookmarks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"discodrive/internal/db"
	"discodrive/internal/storage"
)

func bootstrap(t *testing.T) (*Service, *db.Queries, pgtype.UUID, string) {
	t.Helper()
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Skipf("need Docker: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })
	dsn, _ := pgC.ConnectionString(ctx, "sslmode=disable")
	if err := db.MigrateUp(dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	tenant, _ := q.CreateTenant(ctx, "t")
	u, err := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "u@x", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	root := t.TempDir()
	svc := NewService(pool, q, storage.NewLocalDisk(root))
	svc.Client = &http.Client{}
	svc.Validate = func(string) error { return nil }
	return svc, q, u.ID, root
}

// site serves an HTML page with a title and an icon link, plus the icon itself.
func site(t *testing.T, withIconLink bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			link := ""
			if withIconLink {
				link = `<link rel="shortcut icon" href="/static/icon.png">`
			}
			_, _ = w.Write([]byte(`<html><head><title> Моя страница </title>` + link + `</head><body>x</body></html>`))
		case "/static/icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNGDATA"))
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write([]byte("ICO"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestEnrichPending covers the favicon/title enrichment tick end-to-end:
// linked icon, /favicon.ico fallback, empty-title recovery, and the
// mark-as-tried semantics for dead pages.
func TestEnrichPending(t *testing.T) {
	svc, q, uid, root := bootstrap(t)
	ctx := context.Background()

	linked := site(t, true)
	fallback := site(t, false)

	mk := func(url, title string) db.BrowserBookmark {
		bm, err := q.CreateBrowserBookmark(ctx, db.CreateBrowserBookmarkParams{
			UserID: uid, Title: title, Url: url,
		})
		if err != nil {
			t.Fatalf("create %s: %v", url, err)
		}
		return bm
	}
	withIcon := mk(linked.URL+"/page", "linked")
	withFallback := mk(fallback.URL+"/page", "")
	dead := mk("http://127.0.0.1:1/nope", "dead")

	if err := svc.EnrichPending(ctx); err != nil {
		t.Fatalf("enrich: %v", err)
	}

	get := func(id pgtype.UUID) db.BrowserBookmark {
		bm, err := q.GetBrowserBookmarkForUser(ctx, db.GetBrowserBookmarkForUserParams{ID: id, UserID: uid})
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		return bm
	}

	// Linked icon: .png stored on disk, seq NOT bumped by the favicon itself.
	b1 := get(withIcon.ID)
	if b1.FaviconExt != ".png" || !b1.FaviconTriedAt.Valid {
		t.Fatalf("linked icon: ext=%q tried=%v", b1.FaviconExt, b1.FaviconTriedAt.Valid)
	}
	if b1.Seq != withIcon.Seq {
		t.Fatalf("favicon fetch must not bump seq: %d → %d", withIcon.Seq, b1.Seq)
	}
	data, err := os.ReadFile(filepath.Join(root, faviconRel(db.UUIDString(uid), db.UUIDString(b1.ID), ".png")))
	if err != nil || string(data) != "PNGDATA" {
		t.Fatalf("favicon file: %v %q", err, data)
	}

	// No <link>: /favicon.ico fallback; the empty title is recovered from the
	// page <title> and that DOES bump seq (browsers must pull it).
	b2 := get(withFallback.ID)
	if b2.FaviconExt != ".ico" {
		t.Fatalf("fallback icon: ext=%q", b2.FaviconExt)
	}
	if b2.Title != "Моя страница" {
		t.Fatalf("recovered title = %q", b2.Title)
	}
	if b2.Seq <= withFallback.Seq {
		t.Fatalf("title recovery must bump seq: %d → %d", withFallback.Seq, b2.Seq)
	}

	// Dead page: marked as tried (no re-fetch storm), no favicon, no title.
	b3 := get(dead.ID)
	if !b3.FaviconTriedAt.Valid || b3.FaviconExt != "" || b3.Title != "dead" {
		t.Fatalf("dead page: %+v", b3)
	}

	// The tick must not pick anything up again.
	left, err := q.ListBrowserBookmarksNeedingFavicon(ctx, 10)
	if err != nil || len(left) != 0 {
		t.Fatalf("nothing must remain pending, got %d (%v)", len(left), err)
	}
}

// TestGCTombstonesRemovesFaviconFiles: physically deleted tombstones take
// their favicon files with them and advance the GC watermark.
func TestGCTombstonesRemovesFaviconFiles(t *testing.T) {
	svc, q, uid, root := bootstrap(t)
	ctx := context.Background()

	linked := site(t, true)
	bm, err := q.CreateBrowserBookmark(ctx, db.CreateBrowserBookmarkParams{UserID: uid, Title: "x", Url: linked.URL + "/page"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.EnrichPending(ctx); err != nil {
		t.Fatalf("enrich: %v", err)
	}
	favPath := filepath.Join(root, faviconRel(db.UUIDString(uid), db.UUIDString(bm.ID), ".png"))
	if _, err := os.Stat(favPath); err != nil {
		t.Fatalf("favicon must exist before GC: %v", err)
	}

	if _, err := svc.Delete(ctx, uid, bm.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Force-expire the tombstone, then GC.
	if _, err := svc.pool.Exec(ctx, "UPDATE browser_bookmarks SET updated_at = now() - interval '100 days' WHERE id=$1", bm.ID); err != nil {
		t.Fatalf("age tombstone: %v", err)
	}
	if err := svc.GCTombstones(ctx); err != nil {
		t.Fatalf("gc: %v", err)
	}
	if _, err := os.Stat(favPath); !os.IsNotExist(err) {
		t.Fatalf("favicon file must be removed by GC, stat err=%v", err)
	}
	state, err := q.GetBookmarkSyncState(ctx, uid)
	if err != nil || state.BookmarkGcSeq == 0 {
		t.Fatalf("gc watermark must advance: %+v (%v)", state, err)
	}
}
