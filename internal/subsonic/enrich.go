package subsonic

import (
	"context"
	"time"

	"discodrive/internal/db"
)

// userMarks holds the caller's stars and ratings, looked up once per request.
// Both maps are keyed by "itemType:uuid" (e.g. "song:550e8400-...").
type userMarks struct {
	starred map[string]string // "type:uuid" -> RFC3339 starred_at
	rating  map[string]int    // "type:uuid" -> rating (1..5)
}

// loadMarks fetches all stars and ratings for the given userID in two queries.
// It is designed to be called once per request. On any error it returns empty maps
// so callers never need to handle errors: missing marks are harmless (items just
// won't show a star or rating).
func (h *Handler) loadMarks(ctx context.Context, userID string) userMarks {
	m := userMarks{
		starred: make(map[string]string),
		rating:  make(map[string]int),
	}

	userUUID, err := db.ParseUUID(userID)
	if err != nil {
		return m
	}

	starRows, err := h.q.StarredItems(ctx, userUUID)
	if err == nil {
		for _, row := range starRows {
			if row.StarredAt.Valid {
				key := row.ItemType + ":" + db.UUIDString(row.ItemID)
				m.starred[key] = row.StarredAt.Time.UTC().Format(time.RFC3339)
			}
		}
	}

	ratingRows, err := h.q.RatingsForUser(ctx, userUUID)
	if err == nil {
		for _, row := range ratingRows {
			key := row.ItemType + ":" + db.UUIDString(row.ItemID)
			m.rating[key] = int(row.Rating)
		}
	}

	return m
}

// starredAt returns the ISO8601 starred_at string for the item, or "" if not starred.
func (m userMarks) starredAt(itemType, uuid string) string {
	return m.starred[itemType+":"+uuid]
}

// ratingOf returns the user's rating for the item (0 if none).
func (m userMarks) ratingOf(itemType, uuid string) int {
	return m.rating[itemType+":"+uuid]
}
