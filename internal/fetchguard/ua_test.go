package fetchguard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClientSendsBrowserUA: outbound fetches must not expose Go's default UA —
// sites like Wikipedia 403 it.
func TestClientSendsBrowserUA(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer srv.Close()
	// The SSRF guard is bypassed on purpose (the test server is on loopback):
	// same uaTransport, but without the Dialer.
	c := &http.Client{Transport: uaTransport{inner: http.DefaultTransport}}
	if _, err := c.Get(srv.URL); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.HasPrefix(got, "Mozilla/5.0") || strings.Contains(got, "Go-http-client") {
		t.Fatalf("User-Agent = %q, want a browser-like one", got)
	}
}
