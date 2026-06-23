package subsonic

import (
	"context"

	"discodrive/internal/db"
)

func init() {
	endpoints["tokenInfo"] = tokenInfo
}

// tokenInfo implements the OpenSubsonic apiKeyAuth-extension endpoint.
// It returns the authenticated user's username. The caller is already
// authenticated by the time we dispatch, so we only resolve the email.
func tokenInfo(h *Handler, c *reqCtx) {
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
	c.ok(map[string]any{"tokenInfo": map[string]any{"username": u.Email}})
}
