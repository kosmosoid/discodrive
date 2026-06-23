package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

// syncSettingsResponse is the shape returned by GET and PUT /me/sync.
type syncSettingsResponse struct {
	Enabled bool           `json:"enabled"`
	Folder  *syncFolderDTO `json:"folder"`
	Epoch   int64          `json:"epoch"`
}

type syncFolderDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) buildSyncSettingsResponse(r *http.Request, ss db.SyncSetting, uid pgtype.UUID) syncSettingsResponse {
	resp := syncSettingsResponse{Enabled: ss.Enabled, Epoch: ss.Epoch}
	if ss.FolderNodeID.Valid {
		if node, err := s.q.GetNodeForUser(r.Context(), db.GetNodeForUserParams{ID: ss.FolderNodeID, UserID: uid}); err == nil {
			resp.Folder = &syncFolderDTO{ID: db.UUIDString(node.ID), Name: node.Name}
		}
	}
	return resp
}

// GET /me/sync — return the caller's sync-scope settings.
func (s *Server) handleGetSyncSettings(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	ss, err := s.q.GetSyncSettings(r.Context(), uid)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, syncSettingsResponse{Enabled: false, Folder: nil, Epoch: 0})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s.buildSyncSettingsResponse(r, ss, uid))
}

// PUT /me/sync {"enabled":bool,"folderNodeId":"<uuid>"|null} — upsert sync-scope settings.
func (s *Server) handlePutSyncSettings(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	var req struct {
		Enabled      bool    `json:"enabled"`
		FolderNodeID *string `json:"folderNodeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	var folderNodeID pgtype.UUID // defaults to invalid (NULL)
	if req.FolderNodeID != nil {
		nid, err := db.ParseUUID(*req.FolderNodeID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid folderNodeId")
			return
		}
		node, err := s.q.GetNodeForUser(r.Context(), db.GetNodeForUserParams{ID: nid, UserID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "folder not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !node.IsDir {
			writeError(w, http.StatusBadRequest, "folderNodeId must refer to a directory")
			return
		}
		folderNodeID = nid
	}

	// Enabling scope requires a target folder (otherwise "scope" is meaningless).
	if req.Enabled && !folderNodeID.Valid {
		writeError(w, http.StatusBadRequest, "folderNodeId required when enabled")
		return
	}

	ss, err := s.q.UpsertSyncSettings(r.Context(), db.UpsertSyncSettingsParams{
		UserID:       uid,
		Enabled:      req.Enabled,
		FolderNodeID: folderNodeID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s.buildSyncSettingsResponse(r, ss, uid))
}

// GET /sync/meta — minimal endpoint for the daemon to poll the current scope epoch.
func (s *Server) handleSyncMeta(w http.ResponseWriter, r *http.Request) {
	uid, err := db.ParseUUID(auth.UserID(r.Context()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	scope, err := s.resolveSyncScope(r.Context(), auth.UserID(r.Context()), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scope_epoch": scope.Epoch})
}

// scopeHeader is the opt-in header the daemon (and the future mobile button-app, which
// runs the same Go sync core) sends so the server applies the user's configured sync
// scope. Browser/web/WebDAV clients do NOT send it and therefore always see the whole
// vault — scoping is daemon-only by design. Not a security boundary: the user owns all
// their files regardless; this only limits what the daemon mirrors locally.
const scopeHeader = "X-Discodrive-Scope"

// scopeRequested reports whether the caller opted into sync-scope (i.e. is the daemon).
func scopeRequested(r *http.Request) bool { return r.Header.Get(scopeHeader) == "1" }

// syncScope is the resolved sync root for a user. RelPrefix is the path under the user's
// root that scopes the feed (e.g. "sync"); "" means whole-vault (scope disabled).
type syncScope struct {
	RelPrefix string
	Epoch     int64
}

func (s *Server) resolveSyncScope(ctx context.Context, uidStr string, uid pgtype.UUID) (syncScope, error) {
	ss, err := s.q.GetSyncSettings(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return syncScope{}, nil
	}
	if err != nil {
		return syncScope{}, err
	}
	scope := syncScope{Epoch: ss.Epoch}
	if ss.Enabled && ss.FolderNodeID.Valid {
		node, err := s.q.GetNodeForUser(ctx, db.GetNodeForUserParams{ID: ss.FolderNodeID, UserID: uid})
		if err != nil {
			return syncScope{}, err
		}
		scope.RelPrefix = userRelPath(uidStr, node.DiskPath.String) // strips "<uid>/"
	}
	return scope, nil
}

// escapeLike escapes LIKE wildcards so folder names containing % or _ match literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
