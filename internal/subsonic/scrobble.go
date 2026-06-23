package subsonic

import (
	"context"

	"discodrive/internal/db"
)

func init() {
	endpoints["scrobble"] = scrobble
}

// scrobble implements the Subsonic scrobble endpoint.
//
// Required param: id (tr-<uuid>).
// Optional param: submission — defaults to "true". When "false", the spec
// treats the call as a "now playing" notification rather than a completed play.
// This implementation records a play_history row for every call where
// submission != "false" and returns ok for both cases (simple, YAGNI).
func scrobble(h *Handler, c *reqCtx) {
	id := c.param("id")
	if id == "" {
		c.fail(ErrMissingParam, "Required parameter 'id' is missing")
		return
	}

	kind, songUUIDStr, ok := decID(id)
	if !ok || kind != "tr" {
		c.fail(ErrNotFound, "Song not found")
		return
	}

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	songUUID, err := db.ParseUUID(songUUIDStr)
	if err != nil {
		c.fail(ErrNotFound, "Song not found")
		return
	}

	ctx := context.Background()

	// Authorize: caller must be able to access the song.
	song, err := h.q.AccessibleSong(ctx, db.AccessibleSongParams{
		UserID: userUUID,
		ID:     songUUID,
	})
	if err != nil {
		c.fail(ErrNotFound, "Song not found")
		return
	}

	// submission=false → "now playing" ping; skip the history insert.
	if c.param("submission") != "false" {
		if err := h.q.InsertPlayHistory(ctx, db.InsertPlayHistoryParams{
			UserID: userUUID,
			SongID: song.ID,
		}); err != nil {
			c.fail(ErrGeneric, "database error")
			return
		}
	}

	c.ok(map[string]any{})
}
