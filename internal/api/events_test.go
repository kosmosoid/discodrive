package api

import (
	"context"
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

func eventsTestDB(t *testing.T) (*pgxpool.Pool, *db.Queries, context.Context) {
	t.Helper()
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
	return pool, db.New(pool), ctx
}

func emitChange(t *testing.T, ctx context.Context, q *db.Queries, userID pgtype.UUID) int64 {
	t.Helper()
	node, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID:   userID,
		Name:     "ev.txt",
		Size:     pgtype.Int8{Int64: 1, Valid: true},
		DiskPath: pgtype.Text{String: "ev.txt", Valid: true},
		Mime:     pgtype.Text{String: "text/plain", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	seq, err := q.NextChangeSeq(ctx, userID)
	if err != nil {
		t.Fatalf("NextChangeSeq: %v", err)
	}
	if _, err := q.AppendChange(ctx, db.AppendChangeParams{
		UserID: userID, NodeID: node.ID, Seq: seq, Op: "create", Version: 1,
	}); err != nil {
		t.Fatalf("AppendChange: %v", err)
	}
	return seq
}

func TestEventHubDeliversSeq(t *testing.T) {
	pool, q, ctx := eventsTestDB(t)
	svc := auth.NewService(pool, auth.NewTokenIssuer("s", time.Hour), nil)
	_, user, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	hub := NewEventHub(pool)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.Run(runCtx)
	time.Sleep(200 * time.Millisecond)

	ch, unsub := hub.Subscribe(db.UUIDString(user.ID))
	defer unsub()

	want := emitChange(t, ctx, q, user.ID)
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("seq: expected %d, got %d", want, got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("event not received within 3s")
	}
}

func TestEventHubIsolatesUsers(t *testing.T) {
	pool, q, ctx := eventsTestDB(t)
	svc := auth.NewService(pool, auth.NewTokenIssuer("s", time.Hour), nil)
	_, userA, _ := svc.Register(ctx, "a@x.test", "password12")
	_, userB, _ := svc.Register(ctx, "b@x.test", "password12")
	hub := NewEventHub(pool)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.Run(runCtx)
	time.Sleep(200 * time.Millisecond)

	chB, unsub := hub.Subscribe(db.UUIDString(userB.ID))
	defer unsub()

	emitChange(t, ctx, q, userA.ID)
	select {
	case got := <-chB:
		t.Fatalf("subscriber B received an event belonging to another user: %d", got)
	case <-time.After(1 * time.Second):
	}
}

func TestFormatSSEEvent(t *testing.T) {
	if got := formatSSEEvent(42); got != "data: {\"seq\":42}\n\n" {
		t.Fatalf("SSE format: %q", got)
	}
}
