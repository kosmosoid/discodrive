// Package bookmarks implements the server side of browser bookmark sync: a
// server-authoritative tree in browser_bookmarks with a per-user monotonic
// change cursor (users.bookmark_seq) and tombstones. Browser extensions push
// local changes and pull the changes feed; the web UI edits the same tree.
package bookmarks

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"discodrive/internal/db"
	"discodrive/internal/fetchguard"
	"discodrive/internal/storage"
)

// Service mutates the bookmark tree. Client and Validate are the SSRF-guarded
// defaults; tests replace them to reach httptest servers on 127.0.0.1.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	st   storage.Storage

	Client   *http.Client
	Validate func(string) error
}

func NewService(pool *pgxpool.Pool, q *db.Queries, st storage.Storage) *Service {
	return &Service{
		pool:     pool,
		q:        q,
		st:       st,
		Client:   fetchguard.NewClient(0),
		Validate: fetchguard.ValidateURL,
	}
}

// Delete tombstones the node and its whole subtree in one transaction (the
// seq bump and the tombstone write must commit together, otherwise a client
// could observe the advanced cursor without the tombstones). Returns the
// number of tombstoned rows; 0 = unknown id / already deleted.
func (s *Service) Delete(ctx context.Context, userID, id pgtype.UUID) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	seq, err := qtx.NextBookmarkSeq(ctx, userID)
	if err != nil {
		return 0, err
	}
	n, err := qtx.TombstoneBrowserBookmarkTree(ctx, db.TombstoneBrowserBookmarkTreeParams{
		UserID: userID,
		ID:     id,
		Seq:    seq,
	})
	if err != nil {
		return 0, err
	}
	return n, tx.Commit(ctx)
}

// BulkItem is one node of a bulk import (initial sync from a browser).
type BulkItem struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id"`
	IsFolder bool    `json:"is_folder"`
	Title    string  `json:"title"`
	URL      string  `json:"url"`
	Position int32   `json:"position"`
}

// BulkImport upserts a whole tree in one transaction with a single seq (LWW:
// existing rows, including tombstones, are overwritten and revived). Returns
// the number of items and the new cursor.
func (s *Service) BulkImport(ctx context.Context, userID pgtype.UUID, items []BulkItem) (int, int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	seq, err := qtx.NextBookmarkSeq(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	for i, it := range items {
		id, err := db.ParseUUID(it.ID)
		if err != nil {
			return 0, 0, fmt.Errorf("item %d: bad id %q", i, it.ID)
		}
		var parent pgtype.UUID
		if it.ParentID != nil && *it.ParentID != "" {
			if parent, err = db.ParseUUID(*it.ParentID); err != nil {
				return 0, 0, fmt.Errorf("item %d: bad parent_id %q", i, *it.ParentID)
			}
		}
		url := it.URL
		if it.IsFolder {
			url = ""
		}
		if err := qtx.UpsertBrowserBookmarkAt(ctx, db.UpsertBrowserBookmarkAtParams{
			ID:       id,
			UserID:   userID,
			ParentID: parent,
			IsFolder: it.IsFolder,
			Title:    it.Title,
			Url:      url,
			Position: it.Position,
			Seq:      seq,
		}); err != nil {
			return 0, 0, fmt.Errorf("item %d: %w", i, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return len(items), seq, nil
}
