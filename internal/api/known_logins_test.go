package api_test

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

func TestKnownLoginsUpsertDetectsNew(t *testing.T) {
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
	user, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "u@x", PasswordHash: "x", Role: "user"})

	ins1, _ := q.UpsertKnownLogin(ctx, db.UpsertKnownLoginParams{UserID: user.ID, Fingerprint: "aaa", UserAgent: "UA1", Ip: "1.1.1.1"})
	if !ins1 {
		t.Fatal("first fingerprint should be inserted=true")
	}
	ins2, _ := q.UpsertKnownLogin(ctx, db.UpsertKnownLoginParams{UserID: user.ID, Fingerprint: "aaa", UserAgent: "UA1", Ip: "2.2.2.2"})
	if ins2 {
		t.Fatal("duplicate fingerprint should not be inserted")
	}
	ins3, _ := q.UpsertKnownLogin(ctx, db.UpsertKnownLoginParams{UserID: user.ID, Fingerprint: "bbb", UserAgent: "UA2", Ip: "3.3.3.3"})
	if !ins3 {
		t.Fatal("new fingerprint should be inserted=true")
	}
}
