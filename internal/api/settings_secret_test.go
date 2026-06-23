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
	"discodrive/internal/secret"
)

func TestSecretStoredEncrypted(t *testing.T) {
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

	cipher, _ := secret.New("0123456789abcdef0123456789abcdef")
	enc, _ := cipher.Encrypt("topsecret")
	if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: "smtp.password", Value: enc, IsSecret: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	row, _ := q.GetSetting(ctx, "smtp.password")
	if row.Value == "topsecret" {
		t.Fatal("password is stored in the DB as plaintext")
	}
	dec, err := cipher.Decrypt(row.Value)
	if err != nil || dec != "topsecret" {
		t.Fatalf("decrypt from DB: %v / %q", err, dec)
	}
}
