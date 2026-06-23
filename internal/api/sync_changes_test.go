package api

import (
	"context"
	"encoding/json"
	"fmt"
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

// 3.1: /sync/changes against a real client — pagination (large deltas must not arrive
// in a single chunk) and the response format with content_hash + size (without the hash
// the client cannot distinguish a real change from a no-op touch).
// Uses a real Bearer token through the real middleware.
func TestSyncChangesPaginatesWithHashAndSize(t *testing.T) {
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

	issuer := auth.NewTokenIssuer("secret", time.Hour)
	svc := auth.NewService(pool, issuer, nil)
	tok, user, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 3 file nodes + change log entries seq 1..3
	for i := 1; i <= 3; i++ {
		node, err := q.CreateNode(ctx, db.CreateNodeParams{
			UserID:      user.ID,
			Name:        fmt.Sprintf("f%d.txt", i),
			Size:        pgtype.Int8{Int64: int64(i * 100), Valid: true},
			ContentHash: pgtype.Text{String: fmt.Sprintf("hash%d", i), Valid: true},
			DiskPath:    pgtype.Text{String: fmt.Sprintf("p/f%d.txt", i), Valid: true},
			Mime:        pgtype.Text{String: "text/plain", Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateNode %d: %v", i, err)
		}
		if _, err := q.AppendChange(ctx, db.AppendChangeParams{
			UserID: user.ID, NodeID: node.ID, Seq: int64(i), Op: "create", Version: 1,
		}); err != nil {
			t.Fatalf("AppendChange %d: %v", i, err)
		}
	}

	h := (&Server{q: q}).handleSyncChanges
	handler := svc.Middleware(http.HandlerFunc(h))

	type changeResp struct {
		Changes []struct {
			Seq         int64  `json:"seq"`
			ContentHash string `json:"content_hash"`
			Size        int64  `json:"size"`
		} `json:"changes"`
		Cursor  int64 `json:"cursor"`
		HasMore bool  `json:"has_more"`
	}
	get := func(url string) changeResp {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s → code %d: %s", url, rec.Code, rec.Body.String())
		}
		var resp changeResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return resp
	}

	// page 1: limit=2 → 2 changes, has_more, cursor=2, hash+size present
	p1 := get("/sync/changes?since=0&limit=2")
	if len(p1.Changes) != 2 {
		t.Fatalf("p1: expected 2 changes, got %d", len(p1.Changes))
	}
	if !p1.HasMore {
		t.Fatalf("p1: expected has_more=true")
	}
	if p1.Cursor != 2 {
		t.Fatalf("p1: expected cursor=2, got %d", p1.Cursor)
	}
	if p1.Changes[0].ContentHash != "hash1" {
		t.Fatalf("p1: expected content_hash=hash1, got %q", p1.Changes[0].ContentHash)
	}
	if p1.Changes[0].Size != 100 {
		t.Fatalf("p1: expected size=100, got %d", p1.Changes[0].Size)
	}

	// page 2: fetch the remainder, has_more=false
	p2 := get(fmt.Sprintf("/sync/changes?since=%d&limit=2", p1.Cursor))
	if len(p2.Changes) != 1 {
		t.Fatalf("p2: expected 1 change, got %d", len(p2.Changes))
	}
	if p2.HasMore {
		t.Fatalf("p2: expected has_more=false")
	}
	if p2.Cursor != 3 {
		t.Fatalf("p2: expected cursor=3, got %d", p2.Cursor)
	}
}
