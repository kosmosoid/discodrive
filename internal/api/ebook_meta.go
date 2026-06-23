package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"discodrive/internal/auth"
	"discodrive/internal/db"
	"discodrive/internal/ebook"
)

func mapEbookMetaErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ebook.ErrNotInEbookFolder), errors.Is(err, ebook.ErrNotBook):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *Server) handleGetEbookMeta(w http.ResponseWriter, r *http.Request) {
	m, err := s.metaEditor.Read(r.Context(), auth.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		mapEbookMetaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handlePutEbookMeta(w http.ResponseWriter, r *http.Request) {
	var m ebook.BookMeta
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.metaEditor.Write(r.Context(), auth.UserID(r.Context()), r.PathValue("id"), m); err != nil {
		mapEbookMetaErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetEbookMeta(w http.ResponseWriter, r *http.Request) {
	if err := s.metaEditor.Reset(r.Context(), auth.UserID(r.Context()), r.PathValue("id")); err != nil {
		mapEbookMetaErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMetaFolderCount(w http.ResponseWriter, r *http.Request) {
	n, err := s.metaEditor.CountFolderBooks(r.Context(), auth.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		mapEbookMetaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"affected": n})
}

func (s *Server) handlePostEbookMetaFolder(w http.ResponseWriter, r *http.Request) {
	var in ebook.BulkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	res, err := s.metaEditor.WriteFolder(r.Context(), auth.UserID(r.Context()), r.PathValue("id"), in)
	if err != nil {
		mapEbookMetaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePostEbookScan(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	uid, err := db.ParseUUID(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token subject")
		return
	}
	es, err := s.q.GetEbookSettings(r.Context(), uid)
	if err != nil || !es.FolderNodeID.Valid {
		writeJSON(w, http.StatusOK, map[string]any{"scanning": false})
		return
	}
	folderID := db.UUIDString(es.FolderNodeID)
	go func() {
		idx := ebook.NewIndexer(s.q, s.storageRoot)
		if _, err := idx.ScanFolder(context.Background(), userID, folderID); err != nil {
			log.Printf("discodrive: ebook-scan user=%s: %v", userID, err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"scanning": true})
}
