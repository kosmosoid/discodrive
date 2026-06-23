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
