package podcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
<channel>
  <title>Test Cast</title>
  <description>desc</description>
  <itunes:image href="http://%s/cover.jpg"/>
  <item>
    <title>Ep 1</title>
    <description>first</description>
    <enclosure url="http://%s/ep1.mp3" type="audio/mpeg" length="123"/>
  </item>
</channel></rss>`

func TestFetchFeedAndDownload(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		host := srv.Listener.Addr().String()
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(fmtRSS(host)))
	})
	mux.HandleFunc("/ep1.mp3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("FAKEAUDIO"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	// The SSRF guard blocks 127.0.0.1, so feed/download tests must bypass
	// ValidateURL by calling the parse/download primitives with an unguarded
	// client. Use the *Unsafe variants (see implementation) for tests.
	feed, err := FetchFeedUnsafe(context.Background(), http.DefaultClient, srv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFeedUnsafe: %v", err)
	}
	if feed.Title != "Test Cast" || len(feed.Episodes) != 1 {
		t.Fatalf("feed=%+v", feed)
	}
	if feed.Episodes[0].Title != "Ep 1" || feed.Episodes[0].AudioURL == "" {
		t.Fatalf("ep=%+v", feed.Episodes[0])
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "ep1.mp3")
	size, ct, _, err := DownloadToUnsafe(context.Background(), http.DefaultClient, feed.Episodes[0].AudioURL, dest)
	if err != nil {
		t.Fatalf("DownloadToUnsafe: %v", err)
	}
	if size != int64(len("FAKEAUDIO")) {
		t.Errorf("size=%d", size)
	}
	if ct != "audio/mpeg" {
		t.Errorf("ct=%q", ct)
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "FAKEAUDIO" {
		t.Errorf("file=%q", string(b))
	}
}

// fmtRSS fills both host placeholders in sampleRSS.
func fmtRSS(host string) string {
	return sprintfTwice(sampleRSS, host)
}

func sprintfTwice(tmpl, host string) string {
	return strings.Replace(tmpl, "%s", host, -1)
}
