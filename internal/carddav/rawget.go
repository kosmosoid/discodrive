package carddav

import (
	"net/http"

	"discodrive/internal/db"
)

// ServeRawObject serves a vCard byte-for-byte, BYPASSING go-webdav's re-serialization.
// On GET, go-webdav decodes our stored data into a vcard.Card and re-encodes it — this fails
// for cards with photos or unusual fields (encode error after 200 is sent → truncated response,
// "upstream prematurely closed"). Raw passthrough eliminates this and gives true byte-for-byte round-trips.
// Returns true if the request was handled (object found); false → fall through to go-webdav.
func (b *Backend) ServeRawObject(w http.ResponseWriter, r *http.Request) bool {
	_, uri, obj := parsePath(r.URL.Path)
	if uri == "" || obj == "" {
		return false
	}
	ab, err := b.resolveAddressbook(r.Context(), uri)
	if err != nil {
		return false
	}
	data, etag, err := b.svc.GetAddressbookObject(r.Context(), db.UUIDString(ab.ID), obj)
	if err != nil {
		return false // not found / error — let go-webdav return the proper status
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("ETag", `"`+etag+`"`)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return true
	}
	_, _ = w.Write([]byte(data))
	return true
}
