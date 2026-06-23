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

func TestMigration014Music(t *testing.T) {
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

	// Create a tenant and user to work with.
	tenant, err := q.CreateTenant(ctx, "test-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        "music@test.local",
		PasswordHash: "x",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Assert: music_settings table exists but no row yet → ErrNoRows.
	_, err = q.GetMusicSettings(ctx, u.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows for absent music_settings row, got: %v", err)
	}

	// Upsert with enabled=false (the default path, no folder set).
	ms, err := q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  u.ID,
		Enabled: false,
		// FolderNodeID is zero-value pgtype.UUID (Valid=false → NULL in DB).
	})
	if err != nil {
		t.Fatalf("UpsertMusicSettings: %v", err)
	}
	if ms.Enabled {
		t.Fatalf("expected enabled=false after upsert, got true")
	}
	if db.UUIDString(ms.UserID) != db.UUIDString(u.ID) {
		t.Fatalf("user_id mismatch: got %s want %s", db.UUIDString(ms.UserID), db.UUIDString(u.ID))
	}

	// Read it back and confirm the row persists.
	got, err := q.GetMusicSettings(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetMusicSettings after upsert: %v", err)
	}
	if got.Enabled {
		t.Fatalf("expected enabled=false on read-back, got true")
	}
}
