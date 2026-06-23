package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

// diskUsage(path) is declared per-platform (disk_linux.go / disk_other.go):
// on Linux the size is computed in Frsize units; on other OSes, Bsize.

// secretSettingKeys — setting keys whose values are encrypted (is_secret=true).
var secretSettingKeys = map[string]bool{"smtp.password": true}

// getSecret reads and decrypts a secret setting (empty string if not set).
func (s *Server) getSecret(ctx context.Context, key string) (string, error) {
	row, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if row.Value == "" {
		return "", nil
	}
	return s.cipher.Decrypt(row.Value)
}

// getSettingValue reads a non-secret setting (empty string if not set).
func (s *Server) getSettingValue(ctx context.Context, key string) string {
	row, err := s.q.GetSetting(ctx, key)
	if err != nil {
		return ""
	}
	return row.Value
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// GET /admin/overview — disk stats and users with quota and used space.
func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	total, free, err := diskUsage(s.storageRoot)
	if err != nil {
		total, free = 0, 0 // data directory may not exist yet
	}
	rows, err := s.q.ListUsersWithUsage(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	users := make([]map[string]any, 0, len(rows))
	for _, u := range rows {
		var quota *int64
		if u.StorageQuota.Valid {
			q := u.StorageQuota.Int64
			quota = &q
		}
		users = append(users, map[string]any{
			"id": db.UUIDString(u.ID), "email": u.Email, "role": u.Role,
			"quota": quota, "used": u.Used,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disk":  map[string]any{"total": total, "used": total - free, "free": free},
		"users": users,
	})
}

// POST /admin/users {email,password,role,quota?}
func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Quota    *int64 `json:"quota"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := validateCreds(req.Email, req.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	user, err := s.auth.AdminCreateUser(r.Context(), req.Email, req.Password, req.Role, req.Quota)
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email already taken")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusCreated, toUserDTO(user))
	}
}

// PATCH /admin/users/{id} {role,quota?}
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Role  string `json:"role"`
		Quota *int64 `json:"quota"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		writeError(w, http.StatusBadRequest, "role must be admin or user")
		return
	}
	u, err := s.q.UpdateUser(r.Context(), db.UpdateUserParams{ID: uid, StorageQuota: int8Ptr(req.Quota), Role: req.Role})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUserDTO(u))
}

// DELETE /admin/users/{id}
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == auth.UserID(r.Context()) {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	uid, err := db.ParseUUID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.q.DeleteUser(r.Context(), uid); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/settings — non-secret application settings.
func (s *Server) handleAdminListSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListPublicSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, st := range rows {
		out = append(out, map[string]any{"key": st.Key, "value": st.Value})
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /admin/settings {key,value}
func (s *Server) handleAdminPutSetting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeError(w, http.StatusBadRequest, "key and value are required")
		return
	}
	isSecret := secretSettingKeys[req.Key]
	value := req.Value
	if isSecret {
		if !s.cipher.Enabled() {
			writeError(w, http.StatusBadRequest, "secret encryption is not configured (SETTINGS_ENCRYPTION_KEY)")
			return
		}
		enc, err := s.cipher.Encrypt(req.Value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption error")
			return
		}
		value = enc
	}
	updatedBy, _ := db.ParseUUID(auth.UserID(r.Context()))
	if err := s.q.UpsertSetting(r.Context(), db.UpsertSettingParams{
		Key: req.Key, Value: value, IsSecret: isSecret, UpdatedBy: updatedBy,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if isSecret {
		writeJSON(w, http.StatusOK, map[string]any{"key": req.Key, "set": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": req.Key, "value": req.Value})
}

// GET /devices — the caller's own devices.
func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	devs, err := s.q.ListDevicesForUser(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]map[string]any, 0, len(devs))
	for _, d := range devs {
		var lastSeen any
		if d.LastSeenAt.Valid {
			lastSeen = d.LastSeenAt.Time
		}
		out = append(out, map[string]any{
			"id": db.UUIDString(d.ID), "name": d.Name, "kind": d.Kind, "last_seen_at": lastSeen,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /devices/{id} — revoke one of the caller's own devices.
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	did, err := db.ParseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.q.DeleteDevice(r.Context(), db.DeleteDeviceParams{ID: did, UserID: uid}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
