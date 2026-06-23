package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// bootstrapPairingDB spins up a Postgres container, runs migrations, and returns
// a pool + Queries + auth.Service. If Docker is unavailable the test fails via
// t.Fatalf (not Skip — Docker is expected to be present on this machine).
func bootstrapPairingDB(t *testing.T) (*pgxpool.Pool, *db.Queries, *auth.Service) {
	t.Helper()
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatalf("Docker required: %v", err)
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
	issuer := auth.NewTokenIssuer("secret", time.Hour)
	svc := auth.NewService(pool, issuer, nil)
	return pool, q, svc
}

// doPost encodes body as JSON, sets Authorization if bearer is non-empty,
// executes the request through the handler, and decodes the JSON response into a map.
func doPost(handler http.Handler, path, bearer string, body any) (*httptest.ResponseRecorder, map[string]any) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

// doGet issues a GET through the handler with an optional Bearer token.
func doGet(handler http.Handler, path, bearer string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

// TestDevicePairingFullCycle exercises the full device-pairing flow:
// init → poll pending → approve → poll approved → JWT exchange → sync → double consume → revocation.
func TestDevicePairingFullCycle(t *testing.T) {
	ctx := context.Background()
	pool, q, svc := bootstrapPairingDB(t)
	_ = pool

	userTok, _, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	s := &Server{auth: svc, q: q, loginLimiter: newLoginLimiter()}

	// 1. init — daemon requests pairing codes
	recInit, mInit := doPost(http.HandlerFunc(s.handlePairInit), "/pair/init", "", map[string]any{
		"name": "MacBook Кос",
	})
	if recInit.Code != http.StatusCreated {
		t.Fatalf("init: expected 201, got %d: %s", recInit.Code, recInit.Body.String())
	}
	deviceCode, _ := mInit["device_code"].(string)
	userCode, _ := mInit["user_code"].(string)
	if deviceCode == "" {
		t.Fatalf("init: no device_code in response: %v", mInit)
	}
	if userCode == "" {
		t.Fatalf("init: no user_code in response: %v", mInit)
	}

	// 2. poll pending — status should be pending
	recPoll1, mPoll1 := doPost(http.HandlerFunc(s.handlePairToken), "/pair/token", "", map[string]any{
		"device_code": deviceCode,
	})
	if recPoll1.Code != http.StatusOK {
		t.Fatalf("poll pending: expected 200, got %d: %s", recPoll1.Code, recPoll1.Body.String())
	}
	if mPoll1["status"] != "pending" {
		t.Fatalf("poll pending: expected status=pending, got %v", mPoll1["status"])
	}

	// 3. approve — user confirms via the authenticated endpoint with {code}
	approveMux := http.NewServeMux()
	approveMux.Handle("POST /pair/{code}/approve", svc.Middleware(http.HandlerFunc(s.handlePairApprove)))
	recApprove, mApprove := doPost(approveMux, "/pair/"+userCode+"/approve", userTok, map[string]any{
		"name": "MacBook Кос",
	})
	if recApprove.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d: %s", recApprove.Code, recApprove.Body.String())
	}
	if mApprove["ok"] != true {
		t.Fatalf("approve: expected ok=true, got %v", mApprove)
	}

	// 4. poll approved — should now receive a device_token
	recPoll2, mPoll2 := doPost(http.HandlerFunc(s.handlePairToken), "/pair/token", "", map[string]any{
		"device_code": deviceCode,
	})
	if recPoll2.Code != http.StatusOK {
		t.Fatalf("poll approved: expected 200, got %d: %s", recPoll2.Code, recPoll2.Body.String())
	}
	if mPoll2["status"] != "approved" {
		t.Fatalf("poll approved: expected status=approved, got %v", mPoll2["status"])
	}
	deviceToken, _ := mPoll2["device_token"].(string)
	if deviceToken == "" {
		t.Fatalf("poll approved: no device_token in response: %v", mPoll2)
	}

	// 5. exchange device_token → JWT and verify sync access
	jwt, err := svc.DeviceTokenExchange(ctx, deviceToken)
	if err != nil {
		t.Fatalf("DeviceTokenExchange: %v", err)
	}
	if jwt == "" {
		t.Fatal("DeviceTokenExchange: empty JWT")
	}

	syncMux := http.NewServeMux()
	syncMux.Handle("GET /sync/changes", svc.Middleware(http.HandlerFunc(s.handleSyncChanges)))
	recSync, _ := doGet(syncMux, "/sync/changes?since=0", jwt)
	if recSync.Code != http.StatusOK {
		t.Fatalf("sync/changes: expected 200, got %d: %s", recSync.Code, recSync.Body.String())
	}

	// 6. double consume — re-poll with the same device_code → 410
	recPoll3, _ := doPost(http.HandlerFunc(s.handlePairToken), "/pair/token", "", map[string]any{
		"device_code": deviceCode,
	})
	if recPoll3.Code != http.StatusGone {
		t.Fatalf("double consume: expected 410, got %d: %s", recPoll3.Code, recPoll3.Body.String())
	}

	// 7. device revocation → DeviceTokenExchange should return ErrDeviceToken
	dev, err := q.GetDeviceByTokenHash(ctx, pgtype.Text{String: auth.TokenHash(deviceToken), Valid: true})
	if err != nil {
		t.Fatalf("GetDeviceByTokenHash: %v", err)
	}
	if err := q.DeleteDevice(ctx, db.DeleteDeviceParams{ID: dev.ID, UserID: dev.UserID}); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
	_, err = svc.DeviceTokenExchange(ctx, deviceToken)
	if !errors.Is(err, auth.ErrDeviceToken) {
		t.Fatalf("after revocation: expected ErrDeviceToken, got %v", err)
	}
}

// TestPairingApproveBindsToApprover verifies that the device is bound to exactly
// the user who approved the pairing.
func TestPairingApproveBindsToApprover(t *testing.T) {
	ctx := context.Background()
	_, q, svc := bootstrapPairingDB(t)

	userTokA, userA, err := svc.Register(ctx, "a@x.test", "password12")
	if err != nil {
		t.Fatalf("register A: %v", err)
	}

	s := &Server{auth: svc, q: q, loginLimiter: newLoginLimiter(), pollLimiter: newPollLimiter()}

	// init
	recInit, mInit := doPost(http.HandlerFunc(s.handlePairInit), "/pair/init", "", map[string]any{
		"name": "Test Device",
	})
	if recInit.Code != http.StatusCreated {
		t.Fatalf("init: expected 201, got %d: %s", recInit.Code, recInit.Body.String())
	}
	deviceCode, _ := mInit["device_code"].(string)
	userCode, _ := mInit["user_code"].(string)

	// approve using user A's JWT
	approveMux := http.NewServeMux()
	approveMux.Handle("POST /pair/{code}/approve", svc.Middleware(http.HandlerFunc(s.handlePairApprove)))
	recApprove, _ := doPost(approveMux, "/pair/"+userCode+"/approve", userTokA, map[string]any{})
	if recApprove.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d: %s", recApprove.Code, recApprove.Body.String())
	}

	// poll → device_token
	recPoll, mPoll := doPost(http.HandlerFunc(s.handlePairToken), "/pair/token", "", map[string]any{
		"device_code": deviceCode,
	})
	if recPoll.Code != http.StatusOK {
		t.Fatalf("poll: expected 200, got %d: %s", recPoll.Code, recPoll.Body.String())
	}
	deviceToken, _ := mPoll["device_token"].(string)
	if deviceToken == "" {
		t.Fatalf("poll: no device_token in response: %v", mPoll)
	}

	// look up the device by token_hash and verify its owner
	dev, err := q.GetDeviceByTokenHash(ctx, pgtype.Text{String: auth.TokenHash(deviceToken), Valid: true})
	if err != nil {
		t.Fatalf("GetDeviceByTokenHash: %v", err)
	}
	if db.UUIDString(dev.UserID) != db.UUIDString(userA.ID) {
		t.Fatalf("device bound to %s, expected %s", db.UUIDString(dev.UserID), db.UUIDString(userA.ID))
	}
}

// TestPairingExpired verifies that expired pairings are correctly rejected by
// both /pair/token (410) and /pair/{code} (404).
func TestPairingExpired(t *testing.T) {
	ctx := context.Background()
	_, q, svc := bootstrapPairingDB(t)

	userTok, _, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	s := &Server{auth: svc, q: q, loginLimiter: newLoginLimiter()}

	// Generate a device_code and insert a pairing with an already-expired ExpiresAt directly into the DB
	deviceCode, err := auth.NewDeviceCode()
	if err != nil {
		t.Fatalf("NewDeviceCode: %v", err)
	}
	_, err = q.CreatePairing(ctx, db.CreatePairingParams{
		DeviceCodeHash: auth.TokenHash(deviceCode),
		UserCode:       "EXPIRED1",
		ProposedName:   "x",
		Kind:           "desktop",
		ExpiresAt:      pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatalf("CreatePairing: %v", err)
	}

	// POST /pair/token with expired device_code → 410
	recToken, _ := doPost(http.HandlerFunc(s.handlePairToken), "/pair/token", "", map[string]any{
		"device_code": deviceCode,
	})
	if recToken.Code != http.StatusGone {
		t.Fatalf("/pair/token expired: expected 410, got %d: %s", recToken.Code, recToken.Body.String())
	}

	// GET /pair/{code} with expired user_code → 404
	infoMux := http.NewServeMux()
	infoMux.Handle("GET /pair/{code}", svc.Middleware(http.HandlerFunc(s.handlePairInfo)))
	recInfo, _ := doGet(infoMux, "/pair/EXPIRED1", userTok)
	if recInfo.Code != http.StatusNotFound {
		t.Fatalf("/pair/{code} expired: expected 404, got %d: %s", recInfo.Code, recInfo.Body.String())
	}
}
