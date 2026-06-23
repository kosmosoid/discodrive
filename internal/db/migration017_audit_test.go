package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
)

func TestMigration017AuditLog(t *testing.T) {
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

	tenant, _ := q.CreateTenant(ctx, "t")
	u, err := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "a@test.local", PasswordHash: "x", Role: "user"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	for _, ev := range []string{"login.password", "totp.enabled", "login.passkey"} {
		if err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			UserID: u.ID, Event: ev, Ip: "1.2.3.4", UserAgent: "test-agent",
		}); err != nil {
			t.Fatalf("InsertAuditLog %s: %v", ev, err)
		}
	}

	rows, err := q.ListAuditLog(ctx, db.ListAuditLogParams{UserID: u.ID, Limit: 50})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 audit rows, got %d", len(rows))
	}
	// newest first (id DESC)
	if rows[0].Event != "login.passkey" || rows[2].Event != "login.password" {
		t.Fatalf("expected newest-first ordering, got %s..%s", rows[0].Event, rows[2].Event)
	}
	if rows[0].Ip != "1.2.3.4" || rows[0].UserAgent != "test-agent" {
		t.Fatalf("device fields not stored: %+v", rows[0])
	}

	limited, _ := q.ListAuditLog(ctx, db.ListAuditLogParams{UserID: u.ID, Limit: 2})
	if len(limited) != 2 {
		t.Fatalf("limit not honored: got %d", len(limited))
	}
}
