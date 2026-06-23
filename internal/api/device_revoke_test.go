package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

// Deleting a device via the admin panel must revoke its session IMMEDIATELY: the same
// device JWT that was working moments before must return 401 after the devices row is
// deleted (without waiting for TTL expiry).
func TestDeviceTokenRevokedImmediately(t *testing.T) {
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

	svc := auth.NewService(pool, auth.NewTokenIssuer("secret", time.Hour), nil)
	_, user, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// device with token_hash (as after pairing) + exchange for a device JWT
	deviceToken := "kfd_revoketest"
	dev, err := q.CreateDesktopDevice(ctx, db.CreateDesktopDeviceParams{UserID: user.ID, Name: "MacBook"})
	if err != nil {
		t.Fatalf("CreateDesktopDevice: %v", err)
	}
	if err := q.SetDeviceTokenHash(ctx, db.SetDeviceTokenHashParams{
		ID: dev.ID, TokenHash: pgtype.Text{String: auth.TokenHash(deviceToken), Valid: true},
	}); err != nil {
		t.Fatalf("SetDeviceTokenHash: %v", err)
	}
	jwt, err := svc.DeviceTokenExchange(ctx, deviceToken)
	if err != nil {
		t.Fatalf("DeviceTokenExchange: %v", err)
	}

	probe := func() int {
		h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		req := httptest.NewRequest(http.MethodGet, "/me", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// before deletion — token is valid
	if code := probe(); code != http.StatusOK {
		t.Fatalf("live device: expected 200, got %d", code)
	}
	// delete the device (as admin would)
	if err := q.DeleteDevice(ctx, db.DeleteDeviceParams{ID: dev.ID, UserID: dev.UserID}); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
	// the same JWT is immediately invalid
	if code := probe(); code != http.StatusUnauthorized {
		t.Fatalf("after removing the device: expected 401, got %d", code)
	}
}
