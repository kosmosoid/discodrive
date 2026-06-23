package subsonic

import (
	"context"

	"discodrive/internal/db"
)

func init() {
	endpoints["getInternetRadioStations"] = getInternetRadioStations
	endpoints["createInternetRadioStation"] = createInternetRadioStation
	endpoints["updateInternetRadioStation"] = updateInternetRadioStation
	endpoints["deleteInternetRadioStation"] = deleteInternetRadioStation
}

func getInternetRadioStations(h *Handler, c *reqCtx) {
	ctx := context.Background()

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	rows, err := h.q.ListInternetRadioStations(ctx, userUUID)
	if err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}

	stations := make([]any, 0, len(rows))
	for _, r := range rows {
		stations = append(stations, map[string]any{
			"id":          encID("ir", db.UUIDString(r.ID)),
			"name":        r.Name,
			"streamUrl":   r.StreamUrl,
			"homePageUrl": r.HomepageUrl,
		})
	}

	c.ok(map[string]any{
		"internetRadioStations": map[string]any{
			"internetRadioStation": stations,
		},
	})
}

func createInternetRadioStation(h *Handler, c *reqCtx) {
	ctx := context.Background()

	streamUrl := c.param("streamUrl")
	if streamUrl == "" {
		c.fail(ErrMissingParam, "Required parameter 'streamUrl' is missing")
		return
	}
	name := c.param("name")
	if name == "" {
		c.fail(ErrMissingParam, "Required parameter 'name' is missing")
		return
	}
	homepageUrl := c.param("homepageUrl")

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	_, err = h.q.CreateInternetRadioStation(ctx, db.CreateInternetRadioStationParams{
		UserID:      userUUID,
		Name:        name,
		StreamUrl:   streamUrl,
		HomepageUrl: homepageUrl,
	})
	if err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}

	c.ok(map[string]any{})
}

func updateInternetRadioStation(h *Handler, c *reqCtx) {
	ctx := context.Background()

	id := c.param("id")
	if id == "" {
		c.fail(ErrMissingParam, "Required parameter 'id' is missing")
		return
	}
	streamUrl := c.param("streamUrl")
	if streamUrl == "" {
		c.fail(ErrMissingParam, "Required parameter 'streamUrl' is missing")
		return
	}
	name := c.param("name")
	if name == "" {
		c.fail(ErrMissingParam, "Required parameter 'name' is missing")
		return
	}
	homepageUrl := c.param("homepageUrl")

	kind, uuidStr, ok := decID(id)
	if !ok || kind != "ir" {
		c.fail(ErrNotFound, "station not found")
		return
	}

	stationUUID, err := db.ParseUUID(uuidStr)
	if err != nil {
		c.fail(ErrNotFound, "station not found")
		return
	}

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	n, err := h.q.UpdateInternetRadioStation(ctx, db.UpdateInternetRadioStationParams{
		ID:          stationUUID,
		UserID:      userUUID,
		Name:        name,
		StreamUrl:   streamUrl,
		HomepageUrl: homepageUrl,
	})
	if err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}
	if n == 0 {
		c.fail(ErrNotFound, "station not found")
		return
	}

	c.ok(map[string]any{})
}

func deleteInternetRadioStation(h *Handler, c *reqCtx) {
	ctx := context.Background()

	id := c.param("id")
	if id == "" {
		c.fail(ErrMissingParam, "Required parameter 'id' is missing")
		return
	}

	kind, uuidStr, ok := decID(id)
	if !ok || kind != "ir" {
		c.fail(ErrNotFound, "station not found")
		return
	}

	stationUUID, err := db.ParseUUID(uuidStr)
	if err != nil {
		c.fail(ErrNotFound, "station not found")
		return
	}

	userUUID, err := db.ParseUUID(c.userID)
	if err != nil {
		c.fail(ErrGeneric, "invalid user id")
		return
	}

	n, err := h.q.DeleteInternetRadioStation(ctx, db.DeleteInternetRadioStationParams{
		ID:     stationUUID,
		UserID: userUUID,
	})
	if err != nil {
		c.fail(ErrGeneric, "database error")
		return
	}
	if n == 0 {
		c.fail(ErrNotFound, "station not found")
		return
	}

	c.ok(map[string]any{})
}
