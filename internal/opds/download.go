package opds

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"discodrive/internal/db"
)

// download handles GET /opds/download/{bookId}.
//
// It resolves the book (scoped to the authenticated user via AccessibleBook),
// retrieves the underlying file node, and streams the original file bytes with
// full HTTP Range support via serveNodeFile / http.ServeContent.
//
// Security: any unknown or inaccessible book returns 404 — no distinction
// between "not found" and "not yours" to avoid information leakage.
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	bookIDStr := r.PathValue("bookId")
	bookUUID, err := db.ParseUUID(bookIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	userIDStr := userIDFromContext(r.Context())
	userUUID, err := db.ParseUUID(userIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	book, err := h.q.AccessibleBook(ctx, db.AccessibleBookParams{
		UserID: userUUID,
		ID:     bookUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	node, err := h.q.GetNode(ctx, book.NodeID)
	if err != nil || !node.DiskPath.Valid {
		http.NotFound(w, r)
		return
	}

	h.serveNodeFile(w, r, node.DiskPath.String, node.Name, book.ContentType)
}
