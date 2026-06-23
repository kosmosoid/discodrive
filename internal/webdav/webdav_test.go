package webdav_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
	"discodrive/internal/storage"
	kfdav "discodrive/internal/webdav"
)

func setup(t *testing.T) (kfdav.FileSystem, *storage.FileService, string) {
	t.Helper()
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("requires Docker: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })
	dsn, _ := pgC.ConnectionString(ctx, "sslmode=disable")
	if err := db.MigrateUp(dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	tenant, _ := q.CreateTenant(ctx, "t")
	user, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "u@x", PasswordHash: "x", Role: "user"})
	uid := db.UUIDString(user.ID)
	fs := storage.NewFileService(pool, storage.NewLocalDisk(t.TempDir()))
	return kfdav.NewFileSystem(fs, uid), fs, uid
}

func TestReadFile(t *testing.T) {
	ctx := context.Background()
	wfs, fs, uid := setup(t)
	if _, err := fs.Push(ctx, uid, nil, "note.txt", nil, "init", strings.NewReader("hello dav")); err != nil {
		t.Fatalf("push: %v", err)
	}
	f, err := wfs.OpenFile(ctx, "/note.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	if string(b) != "hello dav" {
		t.Fatalf("content: %q", b)
	}
}

func TestPutCreatesVersionedFile(t *testing.T) {
	ctx := context.Background()
	wfs, fs, uid := setup(t)
	wf, err := wfs.OpenFile(ctx, "/up.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open w: %v", err)
	}
	if _, err := io.WriteString(wf, "via dav"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := wf.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	n, err := fs.NodeByPath(ctx, uid, "/up.txt")
	if err != nil {
		t.Fatalf("file missing after PUT: %v", err)
	}
	if n.Version != 1 {
		t.Fatalf("version=%d, expected 1", n.Version)
	}
}

func TestMkdirAndReaddir(t *testing.T) {
	ctx := context.Background()
	wfs, _, _ := setup(t)
	if err := wfs.Mkdir(ctx, "/docs", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	root, err := wfs.OpenFile(ctx, "/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer root.Close()
	infos, err := root.Readdir(0)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(infos) != 1 || infos[0].Name() != "docs" || !infos[0].IsDir() {
		t.Fatalf("root readdir: %+v", infos)
	}
}

func TestRemoveAll(t *testing.T) {
	ctx := context.Background()
	wfs, fs, uid := setup(t)
	if _, err := fs.Push(ctx, uid, nil, "x.txt", nil, "init", strings.NewReader("y")); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := wfs.RemoveAll(ctx, "/x.txt"); err != nil {
		t.Fatalf("removeall: %v", err)
	}
	if _, err := fs.NodeByPath(ctx, uid, "/x.txt"); err == nil {
		t.Fatal("node still exists after DELETE")
	}
}

func TestRename(t *testing.T) {
	ctx := context.Background()
	wfs, fs, uid := setup(t)
	if _, err := fs.Push(ctx, uid, nil, "a.txt", nil, "init", strings.NewReader("z")); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := wfs.Rename(ctx, "/a.txt", "/b.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := fs.NodeByPath(ctx, uid, "/b.txt"); err != nil {
		t.Fatalf("b.txt missing after rename: %v", err)
	}
}

// Finder writes ._name (AppleDouble) and .DS_Store files during WebDAV copies —
// they must not be materialized in the file tree or web UI.
func TestMacJunkNotStored(t *testing.T) {
	ctx := context.Background()
	wfs, fs, uid := setup(t)
	for _, name := range []string{"/._report.pdf", "/.DS_Store"} {
		wf, err := wfs.OpenFile(ctx, name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		if _, err := io.WriteString(wf, "junk"); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := wf.Close(); err != nil {
			t.Fatalf("close %s: %v", name, err)
		}
	}
	roots, err := fs.RootChildren(ctx, uid)
	if err != nil {
		t.Fatalf("RootChildren: %v", err)
	}
	if len(roots) != 0 {
		t.Fatalf("macOS junk files were materialized: %d nodes", len(roots))
	}
	if _, err := wfs.Stat(ctx, "/.DS_Store"); !os.IsNotExist(err) {
		t.Fatalf("Stat(.DS_Store) = %v, expected NotExist", err)
	}
}

// Proves the root cause of the live "rename folder in Finder → -43 / nginx 502" bug:
// golang.org/x/net/webdav returns 502 when the Destination host:port differs from r.Host,
// which happens behind nginx ("Host $host" drops the port, while Finder's Destination keeps it).
func TestMoveDestinationHostMismatch502(t *testing.T) {
	_, svc, uid := setup(t)
	raw := &webdav.Handler{Prefix: "/dav", FileSystem: kfdav.NewFileSystem(svc, uid), LockSystem: webdav.NewMemLS()}
	do := func(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil) // req.Host = "example.com" (no port)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(raw, "MKCOL", "/dav/music", nil); rec.Code != http.StatusCreated {
		t.Fatalf("mkcol music: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(raw, "MKCOL", "/dav/music/a", nil); rec.Code != http.StatusCreated {
		t.Fatalf("mkcol a: %d %s", rec.Code, rec.Body.String())
	}
	// Destination host carries a port (like Finder behind nginx) → host != r.Host → 502.
	rec := do(raw, "MOVE", "/dav/music/a/", map[string]string{"Destination": "http://example.com:8080/dav/music/b", "Overwrite": "F"})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 from raw handler on host:port mismatch, got %d (%s)", rec.Code, rec.Body.String())
	}
}
