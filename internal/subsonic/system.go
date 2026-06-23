package subsonic

func init() {
	endpoints["ping"] = ping
	endpoints["getLicense"] = getLicense
	endpoints["getOpenSubsonicExtensions"] = getOpenSubsonicExtensions
}

func ping(h *Handler, c *reqCtx) {
	c.ok(map[string]any{})
}

func getLicense(h *Handler, c *reqCtx) {
	c.ok(map[string]any{
		"license": map[string]any{"valid": true},
	})
}

func getOpenSubsonicExtensions(h *Handler, c *reqCtx) {
	c.ok(map[string]any{
		"openSubsonicExtensions": []map[string]any{
			{"name": "formPost", "versions": []int{1}},
			{"name": "apiKeyAuth", "versions": []int{1}},
			{"name": "songLyrics", "versions": []int{1}},
		},
	})
}
