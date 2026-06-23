package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
)

// 000021: sync_settings holds the per-user sync scope (enabled + folder) and an epoch
// counter that bumps whenever the scope changes (so daemons can detect and reconcile).
func TestMigration021SyncSettings(t *testing.T) {
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
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

	tenant, err := q.CreateTenant(ctx, "test-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        "sync@test.local",
		PasswordHash: "x",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Absent row → ErrNoRows.
	if _, err := q.GetSyncSettings(ctx, u.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows for absent sync_settings row, got: %v", err)
	}

	// First upsert (enabled=false, no folder) → epoch starts at 1.
	ss, err := q.UpsertSyncSettings(ctx, db.UpsertSyncSettingsParams{UserID: u.ID, Enabled: false})
	if err != nil {
		t.Fatalf("UpsertSyncSettings: %v", err)
	}
	if ss.Epoch != 1 {
		t.Fatalf("expected epoch=1 on first insert, got %d", ss.Epoch)
	}

	// Re-upsert with the SAME scope → epoch must NOT bump.
	ss, err = q.UpsertSyncSettings(ctx, db.UpsertSyncSettingsParams{UserID: u.ID, Enabled: false})
	if err != nil {
		t.Fatalf("UpsertSyncSettings (same): %v", err)
	}
	if ss.Epoch != 1 {
		t.Fatalf("expected epoch to stay 1 on no-op upsert, got %d", ss.Epoch)
	}

	// Change the scope (enabled true) → epoch bumps to 2.
	ss, err = q.UpsertSyncSettings(ctx, db.UpsertSyncSettingsParams{UserID: u.ID, Enabled: true})
	if err != nil {
		t.Fatalf("UpsertSyncSettings (changed): %v", err)
	}
	if ss.Epoch != 2 {
		t.Fatalf("expected epoch=2 after scope change, got %d", ss.Epoch)
	}
}
