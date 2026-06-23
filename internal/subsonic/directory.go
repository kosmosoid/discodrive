package subsonic

import (
	"context"

	"discodrive/internal/db"
)

func init() {
	endpoints["getMusicDirectory"] = getMusicDirectory
}

// getMusicDirectory maps the Subsonic folder-browse API onto the tag tree.
// id="0" (or empty) → root "Music" directory with artists as children.
// id="ar-<uuid>"    → artist directory with albums as children.
// id="al-<uuid>"    → album directory with songs as children.
func getMusicDirectory(h *Handler, c *reqCtx) {
	ctx := context.Background()
	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	id := c.param("id")
	marks := h.loadMarks(ctx, c.userID)

	// Root: id "0" or empty.
	if id == "" || id == "0" {
		artists, err := h.q.AccessibleArtists(ctx, userUUID)
		if err != nil {
			c.fail(ErrGeneric, "database error")
			return
		}

		children := make([]any, 0, len(artists))
		for _, a := range artists {
			aUUID := db.UUIDString(a.ID)
			child := map[string]any{
				"id":       encID("ar", aUUID),
				"title":    a.Name,
				"isDir":    true,
				"coverArt": encID("ar", aUUID),
				"parent":   "0",
			}
			if s := marks.starredAt("artist", aUUID); s != "" {
				child["starred"] = s
			}
			if r := marks.ratingOf("artist", aUUID); r > 0 {
				child["userRating"] = r
			}
			children = append(children, child)
		}

		c.ok(map[string]any{
			"directory": map[string]any{
				"id":    "0",
				"name":  "Music",
				"child": children,
			},
		})
		return
	}

	kind, uuid, ok := decID(id)
	if !ok {
		c.fail(ErrNotFound, "directory not found")
		return
	}

	switch kind {
	case "ar":
		artistUUID, err := db.ParseUUID(uuid)
		if err != nil {
			c.fail(ErrNotFound, "directory not found")
			return
		}

		artist, err := h.q.AccessibleArtist(ctx, db.AccessibleArtistParams{
			UserID: userUUID,
			ID:     artistUUID,
		})
		if err != nil {
			c.fail(ErrNotFound, "directory not found")
			return
		}

		albums, err := h.q.AccessibleAlbumsByArtist(ctx, db.AccessibleAlbumsByArtistParams{
			UserID:   userUUID,
			ArtistID: artistUUID,
		})
		if err != nil {
			c.fail(ErrGeneric, "database error")
			return
		}

		children := make([]any, 0, len(albums))
		for _, al := range albums {
			alUUID := db.UUIDString(al.ID)
			obj := map[string]any{
				"id":       encID("al", alUUID),
				"title":    al.Name,
				"isDir":    true,
				"coverArt": encID("al", alUUID),
				"parent":   id,
				"artist":   artist.Name,
			}
			if al.Year.Valid {
				obj["year"] = al.Year.Int32
			}
			if s := marks.starredAt("album", alUUID); s != "" {
				obj["starred"] = s
			}
			if r := marks.ratingOf("album", alUUID); r > 0 {
				obj["userRating"] = r
			}
			children = append(children, obj)
		}

		c.ok(map[string]any{
			"directory": map[string]any{
				"id":    id,
				"name":  artist.Name,
				"child": children,
			},
		})

	case "al":
		albumUUID, err := db.ParseUUID(uuid)
		if err != nil {
			c.fail(ErrNotFound, "directory not found")
			return
		}

		album, err := h.q.AccessibleAlbum(ctx, db.AccessibleAlbumParams{
			UserID: userUUID,
			ID:     albumUUID,
		})
		if err != nil {
			c.fail(ErrNotFound, "directory not found")
			return
		}

		artistName, _ := h.q.GetArtistName(ctx, album.ArtistID)

		songs, err := h.q.AccessibleSongsByAlbum(ctx, db.AccessibleSongsByAlbumParams{
			UserID:  userUUID,
			AlbumID: albumUUID,
		})
		if err != nil {
			c.fail(ErrGeneric, "database error")
			return
		}

		children := make([]any, 0, len(songs))
		for _, s := range songs {
			sUUID := db.UUIDString(s.ID)
			children = append(children, buildSongChild(s, album.Name, artistName, album.Year,
				marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID)))
		}

		c.ok(map[string]any{
			"directory": map[string]any{
				"id":    id,
				"name":  album.Name,
				"child": children,
			},
		})

	default:
		c.fail(ErrNotFound, "directory not found")
	}
}
