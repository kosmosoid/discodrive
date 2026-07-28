package saved

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

func TestClaimIsExclusive(t *testing.T) {
	_, pool, q, uid, _ := bootstrap(t, 0)
	ctx := context.Background()
	item, err := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: uid, Url: "https://example.com/x", Kind: KindDownload})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	n1, _ := q.ClaimSavedItem(ctx, item.ID)
	n2, _ := q.ClaimSavedItem(ctx, item.ID)
	if n1 != 1 || n2 != 0 {
		t.Fatalf("claim rows = %d, %d — want 1 then 0", n1, n2)
	}
	_ = pool
}

func TestRecoverStale(t *testing.T) {
	svc, pool, q, uid, _ := bootstrap(t, 0)
	ctx := context.Background()
	item, _ := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: uid, Url: "https://example.com/x", Kind: KindDownload})
	if _, err := pool.Exec(ctx, "UPDATE saved_items SET status='processing', bytes_done=42 WHERE id=$1", item.ID); err != nil {
		t.Fatalf("set processing: %v", err)
	}
	if err := svc.RecoverStale(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	got, _ := q.GetSavedItemForUser(ctx, db.GetSavedItemForUserParams{ID: item.ID, UserID: uid})
	if got.Status != StatusPending || got.BytesDone != 0 {
		t.Fatalf("after recover: status=%q bytes_done=%d, want pending/0", got.Status, got.BytesDone)
	}
}

func TestRetryTransitions(t *testing.T) {
	_, pool, q, uid, _ := bootstrap(t, 0)
	ctx := context.Background()
	item, _ := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: uid, Url: "https://example.com/x", Kind: KindDownload})

	// pending is not retryable (it is already queued).
	if n, _ := q.RetrySavedItem(ctx, db.RetrySavedItemParams{ID: item.ID, UserID: uid}); n != 0 {
		t.Fatalf("retry of pending returned %d rows, want 0", n)
	}
	if _, err := pool.Exec(ctx, "UPDATE saved_items SET status='error', error_msg='boom' WHERE id=$1", item.ID); err != nil {
		t.Fatalf("set error: %v", err)
	}
	if n, _ := q.RetrySavedItem(ctx, db.RetrySavedItemParams{ID: item.ID, UserID: uid}); n != 1 {
		t.Fatal("retry of an errored item must succeed")
	}
	got, _ := q.GetSavedItemForUser(ctx, db.GetSavedItemForUserParams{ID: item.ID, UserID: uid})
	if got.Status != StatusPending || got.ErrorMsg != "" {
		t.Fatalf("after retry: status=%q error=%q, want pending with no error", got.Status, got.ErrorMsg)
	}
}

// TestCleanupDownloads verifies the quiet-queue TTLs: finished download rows
// are removed once stale, fresh ones and articles are kept.
func TestCleanupDownloads(t *testing.T) {
	svc, pool, q, uid, _ := bootstrap(t, 0)
	ctx := context.Background()

	mk := func(url, kind, status, age string) pgtype.UUID {
		it, err := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: uid, Url: url, Kind: kind})
		if err != nil {
			t.Fatalf("upsert %s: %v", url, err)
		}
		if _, err := pool.Exec(ctx,
			"UPDATE saved_items SET status=$2, updated_at=now()-$3::interval WHERE id=$1",
			it.ID, status, age); err != nil {
			t.Fatalf("age %s: %v", url, err)
		}
		return it.ID
	}
	staleDone := mk("https://x/1", KindDownload, "done", "2 hours")
	freshDone := mk("https://x/2", KindDownload, "done", "10 minutes")
	staleErr := mk("https://x/3", KindDownload, "error", "8 days")
	freshErr := mk("https://x/4", KindDownload, "error", "1 day")
	article := mk("https://x/5", KindArticle, "done", "30 days")

	if err := svc.CleanupDownloads(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	exists := func(id pgtype.UUID) bool {
		_, err := q.GetSavedItemForUser(ctx, db.GetSavedItemForUserParams{ID: id, UserID: uid})
		return err == nil
	}
	if exists(staleDone) || exists(staleErr) {
		t.Fatal("stale download rows must be removed")
	}
	if !exists(freshDone) || !exists(freshErr) || !exists(article) {
		t.Fatal("fresh downloads and articles must be kept")
	}
}

// TestProcessPending verifies the worker-tick path: an item inserted as pending
// (bypassing Create/Kickoff, as after a server restart) gets picked up and processed.
func TestProcessPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	svc, _, q, uid, _ := bootstrap(t, 0)
	ctx := context.Background()
	item, _ := q.UpsertSavedItem(ctx, db.UpsertSavedItemParams{UserID: uid, Url: srv.URL + "/f.txt", Kind: KindDownload})
	if err := svc.ProcessPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}
	waitStatus(t, q, item.ID, uid, StatusDone)
}
