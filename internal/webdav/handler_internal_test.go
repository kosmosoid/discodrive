package webdav

import (
	"net/http/httptest"
	"os"
	"testing"

	"discodrive/internal/storage"
)

// A "not found" must map to a *os.PathError, not a bare os.ErrNotExist. x/net/webdav SKIPS
// a *os.PathError child during a PROPFIND walk (so one unresolvable entry — listed by
// RootChildren but not findable by NodeByPath — doesn't abort the whole directory listing
// and trigger a superfluous WriteHeader), while os.IsNotExist still yields 404 for a direct
// request to a missing path.
func TestMapErrNotFoundIsSkippablePathError(t *testing.T) {
	err := mapErr(storage.ErrNotFound)
	if !os.IsNotExist(err) {
		t.Fatalf("mapErr(ErrNotFound) must satisfy os.IsNotExist, got %v", err)
	}
	if _, ok := err.(*os.PathError); !ok {
		t.Fatalf("mapErr(ErrNotFound) must be *os.PathError (so webdav skips it mid-walk), got %T", err)
	}
}

// Verifies the MOVE/COPY Destination normalization that fixes the reverse-proxy
// "host:port mismatch → 502" bug. httptest.NewRequest sets r.Host = "example.com".
func TestNormalizeDestinationHost(t *testing.T) {
	cases := []struct {
		name string
		dest string
		want string
	}{
		{"same host with port → path only", "http://example.com:8080/dav/music/b", "/dav/music/b"},
		{"encoded path preserved", "http://example.com:8080/dav/music/Bi%202", "/dav/music/Bi%202"},
		{"foreign host untouched", "http://other.example.net/dav/y", "http://other.example.net/dav/y"},
		{"path-only untouched", "/dav/music/b", "/dav/music/b"},
		{"empty untouched", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("MOVE", "/dav/music/a/", nil)
			if c.dest != "" {
				r.Header.Set("Destination", c.dest)
			}
			normalizeDestinationHost(r)
			if got := r.Header.Get("Destination"); got != c.want {
				t.Fatalf("Destination = %q, want %q", got, c.want)
			}
		})
	}
}
