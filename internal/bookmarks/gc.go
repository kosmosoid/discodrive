package bookmarks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// tombstoneRetention is how long tombstones are kept for clients to pull.
// A browser that has not synced for longer must full-resync (the GC watermark
// users.bookmark_gc_seq tells it so).
const tombstoneRetention = 90 * 24 * time.Hour

// GCTombstones is the worker tick job physically deleting old tombstones,
// their favicon files, and advancing each affected user's GC watermark.
func (s *Service) GCTombstones(ctx context.Context) error {
	rows, err := s.q.GCBrowserBookmarkTombstones(ctx, pgtype.Timestamptz{
		Time:  time.Now().Add(-tombstoneRetention),
		Valid: true,
	})
	if err != nil {
		return err
	}
	maxSeq := map[pgtype.UUID]int64{}
	for _, r := range rows {
		if r.FaviconExt != "" {
			_ = s.st.Remove(faviconRel(db.UUIDString(r.UserID), db.UUIDString(r.ID), r.FaviconExt))
		}
		if r.Seq > maxSeq[r.UserID] {
			maxSeq[r.UserID] = r.Seq
		}
	}
	for uid, seq := range maxSeq {
		if err := s.q.BumpBookmarkGCSeq(ctx, db.BumpBookmarkGCSeqParams{
			ID:            uid,
			BookmarkGcSeq: seq,
		}); err != nil {
			return err
		}
	}
	return nil
}
