package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"discodrive/internal/db"
)

// TestMigration002SavedItems verifies migration 000002_saved_items: schema
// constraints, the upsert that must NOT reset status, and the user cascade.
func TestMigration002SavedItems(t *testing.T) {
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

	// Insert + upsert: the second save of the same (url, kind) must not reset
	// the status or create a second row.
	it, err := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: u.ID, Url: "https://example.com/a", Kind: "download"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if it.Status != "pending" {
		t.Fatalf("fresh item status = %q, want pending", it.Status)
	}
	if _, err := pool.Exec(ctx, "UPDATE saved_items SET status='done' WHERE id=$1", it.ID); err != nil {
		t.Fatalf("set done: %v", err)
	}
	again, err := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: u.ID, Url: "https://example.com/a", Kind: "download"})
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if again.ID != it.ID {
		t.Fatal("re-save created a new row instead of updating the existing one")
	}
	if again.Status != "done" {
		t.Fatalf("re-save reset status to %q, want done", again.Status)
	}
	// Same URL with a different kind is a separate item.
	other, err := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: u.ID, Url: "https://example.com/a", Kind: "article"})
	if err != nil {
		t.Fatalf("other kind: %v", err)
	}
	if other.ID == it.ID {
		t.Fatal("different kind must be a separate row")
	}

	// CHECK constraints reject unknown kinds — including the retired 'bookmark'
	// (browser bookmarks live in their own table now).
	if _, err := pool.Exec(ctx, "INSERT INTO saved_items (user_id, url, kind) VALUES ($1, 'https://x', 'torrent')", u.ID); err == nil {
		t.Fatal("kind CHECK must reject 'torrent'")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO saved_items (user_id, url, kind) VALUES ($1, 'https://x', 'bookmark')", u.ID); err == nil {
		t.Fatal("kind CHECK must reject the retired 'bookmark'")
	}
	if _, err := pool.Exec(ctx, "UPDATE saved_items SET status='paused' WHERE id=$1", it.ID); err == nil {
		t.Fatal("status CHECK must reject 'paused'")
	}

	// Deleting the user cascades to saved_items.
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id=$1", u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM saved_items").Scan(&n)
	if n != 0 {
		t.Fatalf("expected cascade to remove saved_items, %d left", n)
	}

	// Down migration drops the table cleanly.
	if err := db.MigrateDown(dsn); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	var exists bool
	_ = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='saved_items')").Scan(&exists)
	if exists {
		t.Fatal("saved_items must be dropped by the down migration")
	}
}
