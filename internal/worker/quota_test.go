package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
)

func TestQuotaCandidatesAndMark(t *testing.T) {
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("Docker required: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })
	dsn, _ := pgC.ConnectionString(ctx, "sslmode=disable")
	if err := db.MigrateUp(dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	pool, _ := pgxpool.New(ctx, dsn)
	t.Cleanup(pool.Close)
	q := db.New(pool)
	tenant, _ := q.CreateTenant(ctx, "t")
	u, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "u@x", PasswordHash: "x", Role: "user"})

	if _, err := pool.Exec(ctx, "UPDATE users SET storage_quota=100, storage_used=95 WHERE id=$1", u.ID); err != nil {
		t.Fatalf("update: %v", err)
	}
	cands, _ := q.ListQuotaCandidates(ctx)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if err := q.MarkQuotaNotified(ctx, u.ID); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if c2, _ := q.ListQuotaCandidates(ctx); len(c2) != 0 {
		t.Fatalf("expected 0 after mark, got %d", len(c2))
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET storage_used=10 WHERE id=$1", u.ID); err != nil {
		t.Fatalf("update2: %v", err)
	}
	if err := q.ClearQuotaNotified(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	var notified pgtype.Timestamptz
	_ = pool.QueryRow(ctx, "SELECT quota_notified_at FROM users WHERE id=$1", u.ID).Scan(&notified)
	if notified.Valid {
		t.Fatal("quota_notified_at must reset to NULL")
	}
}
