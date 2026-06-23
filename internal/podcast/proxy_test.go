package podcast

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyStream(t *testing.T) {
	audio := []byte("FAKEAUDIOBYTES")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		if r.Header.Get("Range") != "" {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		_, _ = w.Write(audio)
	}))
	defer srv.Close()

	ctx := context.Background()

	// Plain GET, no Range.
	t.Run("full", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		committed, err := ProxyStreamUnsafe(ctx, http.DefaultClient, rec, req, srv.URL+"/ep.mp3")
		if err != nil {
			t.Fatalf("ProxyStreamUnsafe: %v", err)
		}
		if !committed {
			t.Errorf("committed = false, want true after a successful stream")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", rec.Code)
		}
		if got := rec.Body.Bytes(); !bytes.Equal(got, audio) {
			t.Errorf("body = %q, want %q", got, audio)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
			t.Errorf("Content-Type = %q, want audio/mpeg", ct)
		}
	})

	// With Range header forwarded upstream.
	t.Run("range", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Range", "bytes=0-3")
		rec := httptest.NewRecorder()
		committed, err := ProxyStreamUnsafe(ctx, http.DefaultClient, rec, req, srv.URL+"/ep.mp3")
		if err != nil {
			t.Fatalf("ProxyStreamUnsafe: %v", err)
		}
		if !committed {
			t.Errorf("committed = false, want true after a successful stream")
		}
		if got := rec.Body.Bytes(); !bytes.Equal(got, audio) {
			t.Errorf("body = %q, want %q", got, audio)
		}
	})
}

// TestProxyStreamUpstream404 verifies that when the upstream returns an error
// status (>= 400), the proxy reports committed=false and writes nothing to the
// client body, so the caller can safely emit its own error response.
func TestProxyStreamUpstream404(t *testing.T) {
	audio := []byte("FAKEAUDIOBYTES")
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write(audio) // upstream body must NOT leak to the client.
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		committed, err := ProxyStreamUnsafe(context.Background(), http.DefaultClient, rec, req, srv.URL+"/ep.mp3")
		srv.Close()

		if err == nil {
			t.Fatalf("status %d: ProxyStreamUnsafe err = nil, want error", status)
		}
		if committed {
			t.Errorf("status %d: committed = true, want false (nothing written)", status)
		}
		if bytes.Equal(rec.Body.Bytes(), audio) {
			t.Errorf("status %d: client body leaked upstream audio", status)
		}
	}
}

func TestProxyStreamBlocked(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	// Loopback is rejected by the SSRF guard.
	committed, err := ProxyStream(context.Background(), rec, req, "http://127.0.0.1:1/x")
	if err == nil {
		t.Fatal("ProxyStream(loopback) = nil error, want SSRF block")
	}
	if committed {
		t.Error("committed = true, want false: SSRF block writes nothing")
	}
}
