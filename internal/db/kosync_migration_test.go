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

// TestMigration023ReadingProgress verifies that migration 000023_reading_progress applies
// cleanly and that the sqlc-generated UpsertReadingProgress / GetReadingProgress queries
// work end-to-end, including the ON CONFLICT update path and per-user isolation.
func TestMigration023ReadingProgress(t *testing.T) {
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

	// Create a tenant and two users to verify per-user isolation.
	tenant, err := q.CreateTenant(ctx, "kosync-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	u1, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        "reader1@test.local",
		PasswordHash: "x",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser u1: %v", err)
	}
	u2, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        "reader2@test.local",
		PasswordHash: "x",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser u2: %v", err)
	}

	const doc = "AABBCCDDEEFF00112233445566778899" // opaque KOReader document hash

	// --- Absent row returns ErrNoRows ---

	_, err = q.GetReadingProgress(ctx, db.GetReadingProgressParams{
		UserID:   u1.ID,
		Document: doc,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows for absent progress, got: %v", err)
	}

	// --- Insert path via Upsert ---

	rp, err := q.UpsertReadingProgress(ctx, db.UpsertReadingProgressParams{
		UserID:     u1.ID,
		Document:   doc,
		Progress:   "epub/chapter3/p42",
		Percentage: 0.42,
		Device:     "Kindle",
		DeviceID:   "kindle-001",
	})
	if err != nil {
		t.Fatalf("UpsertReadingProgress (insert): %v", err)
	}
	if rp.Progress != "epub/chapter3/p42" {
		t.Fatalf("progress mismatch after insert: got %q", rp.Progress)
	}
	if rp.Percentage != 0.42 {
		t.Fatalf("percentage mismatch after insert: got %v", rp.Percentage)
	}
	if rp.Device != "Kindle" {
		t.Fatalf("device mismatch after insert: got %q", rp.Device)
	}
	if rp.DeviceID != "kindle-001" {
		t.Fatalf("device_id mismatch after insert: got %q", rp.DeviceID)
	}

	// --- GetReadingProgress reads back the same values ---

	got, err := q.GetReadingProgress(ctx, db.GetReadingProgressParams{
		UserID:   u1.ID,
		Document: doc,
	})
	if err != nil {
		t.Fatalf("GetReadingProgress: %v", err)
	}
	if got.Progress != rp.Progress {
		t.Fatalf("GetReadingProgress progress mismatch: got %q want %q", got.Progress, rp.Progress)
	}
	if got.Percentage != rp.Percentage {
		t.Fatalf("GetReadingProgress percentage mismatch: got %v want %v", got.Percentage, rp.Percentage)
	}
	if got.Device != rp.Device {
		t.Fatalf("GetReadingProgress device mismatch: got %q want %q", got.Device, rp.Device)
	}

	// --- Conflict/update path: upsert with new values overwrites the row ---

	rp2, err := q.UpsertReadingProgress(ctx, db.UpsertReadingProgressParams{
		UserID:     u1.ID,
		Document:   doc,
		Progress:   "epub/chapter7/p91",
		Percentage: 0.71,
		Device:     "Kobo",
		DeviceID:   "kobo-007",
	})
	if err != nil {
		t.Fatalf("UpsertReadingProgress (update): %v", err)
	}
	if rp2.Progress != "epub/chapter7/p91" {
		t.Fatalf("progress not updated on conflict: got %q", rp2.Progress)
	}
	if rp2.Percentage != 0.71 {
		t.Fatalf("percentage not updated on conflict: got %v", rp2.Percentage)
	}
	if rp2.Device != "Kobo" {
		t.Fatalf("device not updated on conflict: got %q", rp2.Device)
	}

	// GetReadingProgress reflects the update.
	after, err := q.GetReadingProgress(ctx, db.GetReadingProgressParams{
		UserID:   u1.ID,
		Document: doc,
	})
	if err != nil {
		t.Fatalf("GetReadingProgress after update: %v", err)
	}
	if after.Progress != "epub/chapter7/p91" {
		t.Fatalf("GetReadingProgress progress not refreshed: got %q", after.Progress)
	}

	// --- Per-user isolation: u2 has no progress for the same document ---

	_, err = q.GetReadingProgress(ctx, db.GetReadingProgressParams{
		UserID:   u2.ID,
		Document: doc,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows for u2 on u1's document, got: %v", err)
	}

	// u2 can independently store its own progress for the same document.
	rp3, err := q.UpsertReadingProgress(ctx, db.UpsertReadingProgressParams{
		UserID:     u2.ID,
		Document:   doc,
		Progress:   "epub/intro/p1",
		Percentage: 0.01,
		Device:     "PocketBook",
		DeviceID:   "pb-999",
	})
	if err != nil {
		t.Fatalf("UpsertReadingProgress u2: %v", err)
	}
	if rp3.Progress != "epub/intro/p1" {
		t.Fatalf("u2 progress mismatch: got %q", rp3.Progress)
	}

	// u1's row is unaffected by u2's upsert.
	u1After, err := q.GetReadingProgress(ctx, db.GetReadingProgressParams{
		UserID:   u1.ID,
		Document: doc,
	})
	if err != nil {
		t.Fatalf("GetReadingProgress u1 after u2 upsert: %v", err)
	}
	if u1After.Progress != "epub/chapter7/p91" {
		t.Fatalf("u1 progress unexpectedly changed: got %q", u1After.Progress)
	}
}
