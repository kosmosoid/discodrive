package subsonic

import (
	"context"

	"discodrive/internal/db"
)

func init() {
	endpoints["getUser"] = getUser
}

// getUser returns the authenticated user's roles. Subsonic clients (e.g. Feishin)
// call this on login and read fields like adminRole to configure permissions;
// without it they crash on `undefined.adminRole`. The `username` param is ignored —
// we always describe the authenticated user.
func getUser(h *Handler, c *reqCtx) {
	uid, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user")
		return
	}
	u, err := h.q.GetUserByID(context.Background(), uid)
	if err != nil {
		c.fail(ErrNotFound, "user not found")
		return
	}
	isAdmin := u.Role == "admin"
	c.ok(map[string]any{"user": map[string]any{
		"username":            u.Email,
		"email":               u.Email,
		"scrobblingEnabled":   true,
		"adminRole":           isAdmin,
		"settingsRole":        isAdmin,
		"downloadRole":        true,
		"uploadRole":          false,
		"playlistRole":        true,
		"coverArtRole":        true,
		"commentRole":         false,
		"podcastRole":         false,
		"streamRole":          true,
		"jukeboxRole":         false,
		"shareRole":           false,
		"videoConversionRole": false,
		"folder":              []any{0},
	}})
}
