package subsonic

import (
	"context"
	"log"

	"discodrive/internal/db"
	"discodrive/internal/music"
)

func init() {
	endpoints["getScanStatus"] = getScanStatus
	endpoints["startScan"] = startScan
}

// getScanStatus returns the current library scan state for the authenticated user.
func getScanStatus(h *Handler, c *reqCtx) {
	h.scanMu.Lock()
	st := h.scanState[c.userID]
	var scanning bool
	var count int
	if st != nil {
		scanning = st.scanning
		count = st.count
	}
	h.scanMu.Unlock()

	c.ok(map[string]any{
		"scanStatus": map[string]any{
			"scanning": scanning,
			"count":    count,
		},
	})
}

// startScan triggers a background library scan for the authenticated user.
func startScan(h *Handler, c *reqCtx) {
	ctx := context.Background()

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	settings, err := h.q.GetMusicSettings(ctx, userUUID)
	if err != nil {
		c.fail(ErrGeneric, "could not load music settings")
		return
	}
	if !settings.FolderNodeID.Valid {
		// No music folder configured — nothing to scan.
		c.ok(map[string]any{
			"scanStatus": map[string]any{
				"scanning": false,
				"count":    0,
			},
		})
		return
	}

	h.scanMu.Lock()
	st, ok := h.scanState[c.userID]
	if !ok {
		st = &scanInfo{}
		h.scanState[c.userID] = st
	}
	if st.scanning {
		// Scan already in progress — return current state.
		count := st.count
		h.scanMu.Unlock()
		c.ok(map[string]any{
			"scanStatus": map[string]any{
				"scanning": true,
				"count":    count,
			},
		})
		return
	}
	st.scanning = true
	st.count = 0
	h.scanMu.Unlock()

	userID := c.userID
	folderID := db.UUIDString(settings.FolderNodeID)

	go func() {
		idx := music.NewIndexer(h.q, h.storageRoot)
		n, err := idx.ScanFolder(context.Background(), userID, folderID)
		if err != nil {
			log.Printf("discodrive: music-scan user=%s: %v", userID, err)
		}
		h.scanMu.Lock()
		st.scanning = false
		st.count = n
		h.scanMu.Unlock()
	}()

	c.ok(map[string]any{
		"scanStatus": map[string]any{
			"scanning": true,
			"count":    0,
		},
	})
}
