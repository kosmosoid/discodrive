package api

import (
	"encoding/json"
	"net/http"
	"time"

	"discodrive/internal/auth"
	"discodrive/internal/dav"
	"discodrive/internal/db"
)

// POST /me/calendars/{id}/share {email, expires_in_seconds?}
func (s *Server) handleShareCalendar(w http.ResponseWriter, r *http.Request) {
	owner := auth.UserID(r.Context())
	calID := r.PathValue("id")
	var body struct {
		Email            string `json:"email"`
		ExpiresInSeconds int64  `json:"expires_in_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	var exp *time.Time
	if body.ExpiresInSeconds > 0 {
		t := time.Now().Add(time.Duration(body.ExpiresInSeconds) * time.Second)
		exp = &t
	}
	share, err := s.dav.ShareCalendar(r.Context(), owner, calID, body.Email, exp)
	switch err {
	case nil:
	case dav.ErrNotOwner:
		writeError(w, http.StatusForbidden, "only the owner can share")
		return
	case dav.ErrNotFound:
		writeError(w, http.StatusNotFound, "calendar or user not found")
		return
	default:
		writeError(w, http.StatusInternalServerError, "failed to share")
		return
	}
	// notify the recipient (best-effort, same as for file shares)
	calName := ""
	if c, e := s.dav.GetCalendar(r.Context(), calID); e == nil {
		calName = c.Name
	}
	sharerEmail := ""
	if su, e := s.q.GetUserByID(r.Context(), mustUUID(owner)); e == nil {
		sharerEmail = su.Email
	}
	s.notify.Emit(r.Context(), db.UUIDString(share.SharedWithUser), "share.received",
		map[string]any{"NodeName": calName, "SharerEmail": sharerEmail, "ResourceLabel": "calendar"})
	writeJSON(w, http.StatusCreated, map[string]any{"share_id": db.UUIDString(share.ID)})
}

// GET /me/calendars/{id}/shares
func (s *Server) handleListCalendarShares(w http.ResponseWriter, r *http.Request) {
	infos, err := s.dav.ListCalendarShares(r.Context(), auth.UserID(r.Context()), r.PathValue("id"))
	switch err {
	case nil:
	case dav.ErrNotOwner:
		writeError(w, http.StatusForbidden, "owner only")
		return
	case dav.ErrNotFound:
		writeError(w, http.StatusNotFound, "calendar not found")
		return
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	type shareDTO struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		ExpiresAt string `json:"expires_at,omitempty"`
	}
	out := make([]shareDTO, 0, len(infos))
	for _, i := range infos {
		out = append(out, shareDTO{ID: i.ID, Email: i.Email, ExpiresAt: i.ExpiresAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /me/calendars/{id}/shares/{shareId}
func (s *Server) handleDeleteCalendarShare(w http.ResponseWriter, r *http.Request) {
	err := s.dav.DeleteCalendarShare(r.Context(), auth.UserID(r.Context()), r.PathValue("shareId"))
	switch err {
	case nil:
		w.WriteHeader(http.StatusNoContent)
	case dav.ErrNotOwner:
		writeError(w, http.StatusForbidden, "owner only")
	case dav.ErrNotFound:
		writeError(w, http.StatusNotFound, "share not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
