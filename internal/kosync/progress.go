package kosync

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"discodrive/internal/db"
)

// usersAuth handles GET /users/auth.
// Returns 200 {"username": email} on success, 401 {} on failure.
func (h *Handler) usersAuth(w http.ResponseWriter, r *http.Request) {
	_, email, ok := h.authUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, struct{}{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": email})
}

// usersCreate handles POST /users/create.
// KOReader sends this on first connect; we reuse auth to check if the user exists.
// Returns 201 on success, 402 on failure (KOReader treats 402 as non-fatal "user exists/invalid").
func (h *Handler) usersCreate(w http.ResponseWriter, r *http.Request) {
	_, email, ok := h.authUser(r)
	if !ok {
		writeJSON(w, http.StatusPaymentRequired, struct{}{})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"username": email})
}

// progressBody is the JSON body for PUT /syncs/progress.
type progressBody struct {
	Document   string  `json:"document"`
	Progress   string  `json:"progress"`
	Percentage float64 `json:"percentage"`
	Device     string  `json:"device"`
	DeviceID   string  `json:"device_id"`
}

// syncsProgressPut handles PUT /syncs/progress.
// Upserts the reading position for the authenticated user.
func (h *Handler) syncsProgressPut(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := h.authUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, struct{}{})
		return
	}

	var body progressBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, struct{}{})
		return
	}

	uid, err := db.ParseUUID(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, struct{}{})
		return
	}

	row, err := h.q.UpsertReadingProgress(r.Context(), db.UpsertReadingProgressParams{
		UserID:     uid,
		Document:   body.Document,
		Progress:   body.Progress,
		Percentage: float32(body.Percentage),
		Device:     body.Device,
		DeviceID:   body.DeviceID,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, struct{}{})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"document":  row.Document,
		"timestamp": row.UpdatedAt.Time.Unix(),
	})
}

// syncsProgressGet handles GET /syncs/progress/{document}.
// Returns the stored reading position for the authenticated user, or {} if not found.
func (h *Handler) syncsProgressGet(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := h.authUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, struct{}{})
		return
	}

	document := r.PathValue("document")

	uid, err := db.ParseUUID(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, struct{}{})
		return
	}

	row, err := h.q.GetReadingProgress(r.Context(), db.GetReadingProgressParams{
		UserID:   uid,
		Document: document,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, struct{}{})
			return
		}
		writeJSON(w, http.StatusInternalServerError, struct{}{})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"document":   row.Document,
		"progress":   row.Progress,
		"percentage": row.Percentage,
		"device":     row.Device,
		"device_id":  row.DeviceID,
		"timestamp":  row.UpdatedAt.Time.Unix(),
	})
}
