package api

import (
	"encoding/json"
	"net/http"

	"discodrive/internal/auth"
	"discodrive/internal/db"
	"discodrive/internal/notify"
)

// GET /me/notifications — catalog of optional notification events and the current toggle state (email channel).
func (s *Server) handleGetMyNotifications(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	rows, err := s.q.ListNotificationPrefs(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pref := map[string]bool{}
	for _, p := range rows {
		pref[p.EventKey+"|"+p.Channel] = p.Enabled
	}
	type item struct {
		Key       string `json:"key"`
		Category  string `json:"category"`
		Mandatory bool   `json:"mandatory"`
		Enabled   bool   `json:"enabled"`
	}
	out := make([]item, 0, len(notify.Catalog))
	for _, ev := range notify.Catalog {
		enabled := true
		if v, ok := pref[ev.Key+"|email"]; ok {
			enabled = v
		}
		out = append(out, item{Key: ev.Key, Category: string(ev.Category), Mandatory: ev.Mandatory, Enabled: ev.Mandatory || enabled})
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /me/notifications {event_key, enabled} — toggle a notification event for the email channel.
func (s *Server) handlePutMyNotification(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	var req struct {
		EventKey string `json:"event_key"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventKey == "" {
		writeError(w, http.StatusBadRequest, "event_key is required")
		return
	}
	ev, ok := notify.Catalog[req.EventKey]
	if !ok || ev.Mandatory {
		writeError(w, http.StatusBadRequest, "event cannot be configured")
		return
	}
	if err := s.q.UpsertNotificationPref(r.Context(), db.UpsertNotificationPrefParams{
		UserID: uid, EventKey: req.EventKey, Channel: "email", Enabled: req.Enabled,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event_key": req.EventKey, "enabled": req.Enabled})
}

// GET /admin/smtp — SMTP settings (password omitted; only its presence is indicated).
func (s *Server) handleAdminGetSmtp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pwRow, _ := s.q.GetSetting(ctx, "smtp.password")
	writeJSON(w, http.StatusOK, map[string]any{
		"host":                  s.getSettingValue(ctx, "smtp.host"),
		"port":                  s.getSettingValue(ctx, "smtp.port"),
		"username":              s.getSettingValue(ctx, "smtp.username"),
		"from":                  s.getSettingValue(ctx, "smtp.from"),
		"security":              s.getSettingValue(ctx, "smtp.security"),
		"password_set":          pwRow.Value != "",
		"notifications_enabled": s.getSettingValue(ctx, "notifications.enabled") != "false",
	})
}

// POST /admin/smtp/test — send a test email to the current admin.
func (s *Server) handleAdminSmtpTest(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	s.notify.Emit(r.Context(), uid, "account.profile_changed", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "message queued for delivery; check your inbox and logs if it does not arrive"})
}
