package podcast

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// audioServer serves body at /ep.mp3.
func audioServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// An episode larger than the space left must abort and leave nothing on disk —
// a partial file would keep filling the quota with something nothing can play.
func TestDownloadToLimit_StopsAndCleansUp(t *testing.T) {
	srv := audioServer(t, strings.Repeat("A", 1000))
	dest := filepath.Join(t.TempDir(), "ep.mp3")

	_, _, _, err := DownloadToUnsafe(context.Background(), http.DefaultClient, srv.URL+"/ep.mp3", dest, 100)
	if err == nil {
		t.Fatal("a download past the limit must fail")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("the partial file must be removed, stat = %v", statErr)
	}
}

// A body that ends exactly at the limit fits.
func TestDownloadToLimit_ExactlyAtLimit(t *testing.T) {
	srv := audioServer(t, strings.Repeat("A", 100))
	dest := filepath.Join(t.TempDir(), "ep.mp3")

	size, _, _, err := DownloadToUnsafe(context.Background(), http.DefaultClient, srv.URL+"/ep.mp3", dest, 100)
	if err != nil {
		t.Fatalf("a body ending exactly at the limit must be accepted: %v", err)
	}
	if size != 100 {
		t.Fatalf("size = %d, want 100", size)
	}
}

// An "unlimited" ceiling reaches this code as math.MaxInt64. Adding the usual +1 probe
// byte to it overflows to a negative limit, and io.LimitReader then yields an empty
// body — which stored a 0-byte episode that reported success.
func TestDownloadToLimit_MaxInt64IsNotAnOverflow(t *testing.T) {
	const body = "FAKEAUDIO"
	srv := audioServer(t, body)
	dest := filepath.Join(t.TempDir(), "ep.mp3")

	size, _, _, err := DownloadToUnsafe(context.Background(), http.DefaultClient, srv.URL+"/ep.mp3", dest, math.MaxInt64)
	if err != nil {
		t.Fatalf("DownloadToUnsafe: %v", err)
	}
	if size != int64(len(body)) {
		t.Fatalf("size = %d, want %d", size, len(body))
	}
	b, _ := os.ReadFile(dest)
	if string(b) != body {
		t.Fatalf("file = %q, want %q", b, body)
	}
}
