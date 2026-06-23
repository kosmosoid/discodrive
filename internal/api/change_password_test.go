package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

// Changing the password invalidates previously issued tokens (token_version), but
// returns a fresh working token for the current session. Both outcomes are verified
// through the real middleware.
func TestChangePasswordInvalidatesOldTokens(t *testing.T) {
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

	issuer := auth.NewTokenIssuer("secret", time.Hour)
	svc := auth.NewService(pool, issuer, nil)

	oldTok, user, err := svc.Register(ctx, "u@x.test", "oldpass12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// wrong current password → rejected, version unchanged
	if _, err := svc.ChangePassword(ctx, db.UUIDString(user.ID), "wrongpass", "newpass34"); err != auth.ErrInvalidCreds {
		t.Fatalf("expected ErrInvalidCreds, got %v", err)
	}

	newTok, err := svc.ChangePassword(ctx, db.UUIDString(user.ID), "oldpass12", "newpass34")
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	probe := func(tok string) int {
		h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		req := httptest.NewRequest(http.MethodGet, "/me", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := probe(oldTok); code != http.StatusUnauthorized {
		t.Fatalf("old token after password change: expected 401, got %d", code)
	}
	if code := probe(newTok); code != http.StatusOK {
		t.Fatalf("new token: expected 200, got %d", code)
	}
}
