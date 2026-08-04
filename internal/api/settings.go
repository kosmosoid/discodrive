package api

import (
	"encoding/json"
	"net/http"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

// Supported UI languages. Default is en. Clients (web, DiscoDrive, daemon)
// maintain the same list.
var supportedLanguages = map[string]bool{
	"en": true, "de": true, "uk": true, "fr": true, "es": true, "ru": true, "sr": true,
}

// GET /me/language — the current user's UI language.
func (s *Server) handleGetLanguage(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	lang, err := s.q.GetUserLanguage(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"language": lang})
}

// Bounds for the session lifetime a user may pick for themselves. 0 (never expires) is
// allowed separately; anything shorter than the floor would log people out mid-task, and
// the ceiling is there so a typo cannot produce a nonsense value — "effectively forever"
// already has its own setting.
const (
	minSessionTTLMinutes = 5
	maxSessionTTLMinutes = 365 * 24 * 60
)

// GET /me/session — the current user's session lifetime, in minutes (0 = never expires).
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	ttl, err := s.q.GetUserSessionTTL(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ttl_minutes": ttl})
}

// PUT /me/session {"ttl_minutes":480} — how long a session may stay idle before the
// user has to sign in again; 0 means never. Takes effect on the next request, which
// renews the token with the new lifetime.
func (s *Server) handleSetSession(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	var req struct {
		TTLMinutes *int32 `json:"ttl_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TTLMinutes == nil {
		writeError(w, http.StatusBadRequest, "ttl_minutes is required")
		return
	}
	ttl := *req.TTLMinutes
	if ttl != 0 && (ttl < minSessionTTLMinutes || ttl > maxSessionTTLMinutes) {
		writeError(w, http.StatusBadRequest, "ttl_minutes must be 0 (never) or between 5 and 525600")
		return
	}
	if err := s.q.SetUserSessionTTL(r.Context(), db.SetUserSessionTTLParams{ID: uid, SessionTtlMinutes: ttl}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ttl_minutes": ttl})
}

// PUT /me/language {"language":"ru"} — change the UI language.
func (s *Server) handleSetLanguage(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !supportedLanguages[req.Language] {
		writeError(w, http.StatusBadRequest, "unsupported language")
		return
	}
	if err := s.q.SetUserLanguage(r.Context(), db.SetUserLanguageParams{ID: uid, Language: req.Language}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"language": req.Language})
}
