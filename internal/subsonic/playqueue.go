package subsonic

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

func init() {
	endpoints["savePlayQueue"] = savePlayQueue
	endpoints["getPlayQueue"] = getPlayQueue
}

func savePlayQueue(h *Handler, c *reqCtx) {
	ctx := context.Background()

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	// Resolve current track (optional).
	var currentPg pgtype.UUID // zero value = NULL (Valid=false)
	if cur := c.param("current"); cur != "" {
		kind, uuid, ok := decID(cur)
		if ok && kind == "tr" {
			parsed, err := db.ParseUUID(uuid)
			if err == nil {
				currentPg = parsed
			}
		}
	}

	// Parse position in milliseconds.
	position, _ := strconv.ParseInt(c.param("position"), 10, 64)

	// Clear existing entries then re-insert. Not wrapped in a transaction
	// (consistent with the playlist clear+insert path); a crash mid-write can
	// leave the queue empty with a stale header, self-healed by the next save.
	if err := h.q.ClearPlayQueueEntries(ctx, userUUID); err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}

	// idx increments only for successfully inserted entries (gap-free).
	idx := 0
	for _, rawID := range c.paramList("id") {
		kind, uuid, ok := decID(rawID)
		if !ok || kind != "tr" {
			continue
		}
		songUUID, err := db.ParseUUID(uuid)
		if err != nil {
			continue
		}
		// Skip songs the user cannot access (prevents FK violations from foreign ids).
		if _, err := h.q.AccessibleSong(ctx, db.AccessibleSongParams{UserID: userUUID, ID: songUUID}); err != nil {
			continue
		}
		if err := h.q.AddPlayQueueEntry(ctx, db.AddPlayQueueEntryParams{
			UserID: userUUID,
			Idx:    int32(idx),
			SongID: songUUID,
		}); err != nil {
			c.fail(ErrGeneric, "database error")
			return
		}
		idx++
	}

	if err := h.q.UpsertPlayQueue(ctx, db.UpsertPlayQueueParams{
		UserID:     userUUID,
		CurrentID:  currentPg,
		PositionMs: position,
		ChangedBy:  c.param("c"),
	}); err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}

	c.ok(map[string]any{})
}

func getPlayQueue(h *Handler, c *reqCtx) {
	ctx := context.Background()

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	head, err := h.q.GetPlayQueue(ctx, userUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Empty queue is not an error.
			c.ok(map[string]any{"playQueue": map[string]any{}})
			return
		}
		c.fail(ErrGeneric, "database error")
		return
	}

	entryRows, err := h.q.GetPlayQueueEntries(ctx, userUUID)
	if err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}

	marks := h.loadMarks(ctx, c.userID)

	entries := make([]any, 0, len(entryRows))
	for _, r := range entryRows {
		s := db.Song{
			ID:            r.ID,
			UserID:        r.UserID,
			AlbumID:       r.AlbumID,
			ArtistID:      r.ArtistID,
			NodeID:        r.NodeID,
			Title:         r.Title,
			Track:         r.Track,
			Disc:          r.Disc,
			Duration:      r.Duration,
			Bitrate:       r.Bitrate,
			Suffix:        r.Suffix,
			ContentType:   r.ContentType,
			Size:          r.Size,
			Genre:         r.Genre,
			MusicbrainzID: r.MusicbrainzID,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
		}
		sUUID := db.UUIDString(r.ID)
		child := buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
			marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
		entries = append(entries, child)
	}

	pq := map[string]any{
		"position": head.PositionMs,
		"entry":    entries,
	}
	if head.CurrentID.Valid {
		pq["current"] = encID("tr", db.UUIDString(head.CurrentID))
	}
	if head.ChangedBy != "" {
		pq["changedBy"] = head.ChangedBy
	}
	if head.ChangedAt.Valid {
		pq["changed"] = head.ChangedAt.Time.UTC().Format(time.RFC3339)
	}

	c.ok(map[string]any{"playQueue": pq})
}
