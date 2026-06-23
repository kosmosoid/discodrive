package webdav

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"golang.org/x/net/webdav"

	"discodrive/internal/storage"
)

// Locks are scoped per user: memLS keys locks by path (after stripping the /dav prefix),
// so a shared LockSystem would collide on same-named files of different users
// (user A's LOCK → 423 for user B). One memLS per userID eliminates this.
var (
	lsMu     sync.Mutex
	lsByUser = map[string]webdav.LockSystem{}
)

func userLockSystem(userID string) webdav.LockSystem {
	lsMu.Lock()
	defer lsMu.Unlock()
	ls, ok := lsByUser[userID]
	if !ok {
		ls = webdav.NewMemLS()
		lsByUser[userID] = ls
	}
	return ls
}

// Handler assembles a WebDAV handler: per-request FileSystem and LockSystem
// are scoped to the userID from the context (set by Auth). LockSystem is in-memory
// (class 2 for Finder read-write), one instance per user.
func Handler(svc *storage.FileService, prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := UserID(r.Context())
		normalizeDestinationHost(r)
		h := &webdav.Handler{
			Prefix:     prefix,
			FileSystem: NewFileSystem(svc, uid),
			LockSystem: userLockSystem(uid),
			// Surface unexpected handler errors. Skip os.IsNotExist: macOS Finder probes a
			// flurry of non-existent paths (.Spotlight-V100, eTokenVirtual, …) whose 404s
			// are normal and would otherwise spam the log.
			Logger: func(req *http.Request, err error) {
				if err != nil && !os.IsNotExist(err) {
					log.Printf("webdav: %s %s: %v", req.Method, req.URL.Path, err)
				}
			},
		}
		h.ServeHTTP(w, r)
	})
}

// normalizeDestinationHost rewrites the MOVE/COPY Destination header to a host-relative
// path when it targets the same host as the request. golang.org/x/net/webdav rejects a
// Destination whose host differs from r.Host with 502 Bad Gateway. Behind a reverse proxy
// that drops the port (nginx "proxy_set_header Host $host" yields "host", while the client
// sends "host:port" in Destination), this strict check fails and Finder reports error -43.
// Comparing host names without the port and stripping to the path sidesteps it.
func normalizeDestinationHost(r *http.Request) {
	d := r.Header.Get("Destination")
	if d == "" {
		return
	}
	u, err := url.Parse(d)
	if err != nil || u.Host == "" {
		return
	}
	if sameHostname(u.Host, r.Host) {
		r.Header.Set("Destination", u.EscapedPath())
	}
}

// sameHostname compares two host[:port] strings ignoring the port.
func sameHostname(a, b string) bool {
	if h, _, err := net.SplitHostPort(a); err == nil {
		a = h
	}
	if h, _, err := net.SplitHostPort(b); err == nil {
		b = h
	}
	return strings.EqualFold(a, b)
}
