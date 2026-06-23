// Package podcast fetches and parses podcast RSS feeds and downloads episode
// and cover files. All network access goes through an SSRF guard (see ssrf.go),
// except the *Unsafe variants used by tests with an explicit client.
package podcast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

// Feed is a normalized podcast channel.
type Feed struct {
	Title       string
	Description string
	ImageURL    string
	Episodes    []Episode
}

// Episode is a normalized podcast episode from a feed.
type Episode struct {
	Title       string
	Description string
	PubDate     *time.Time
	AudioURL    string
	Duration    int // seconds, 0 if unknown
	ContentType string
}

// guardedClient is the default SSRF-bounded HTTP client. safeDialer pins the IP
// check to the actual dialed address (DNS-rebinding safe); CheckRedirect also
// re-validates every redirect target so a feed host cannot 302 to a private IP.
func guardedClient() *http.Client {
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: &http.Transport{DialContext: safeDialer.DialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return ValidateURL(req.URL.String())
		},
	}
}

// FetchFeed validates the URL (SSRF guard) then parses the feed.
func FetchFeed(ctx context.Context, feedURL string) (Feed, error) {
	if err := ValidateURL(feedURL); err != nil {
		return Feed{}, err
	}
	return FetchFeedUnsafe(ctx, guardedClient(), feedURL)
}

// FetchFeedUnsafe parses a feed using the given client WITHOUT the SSRF guard.
// WARNING: for tests only — callers are responsible for ensuring the URL is safe.
func FetchFeedUnsafe(ctx context.Context, client *http.Client, feedURL string) (Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return Feed{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Feed{}, err
	}
	defer resp.Body.Close()
	const maxFeedBytes = 16 << 20 // 16 MiB — sufficient for any real feed
	parsed, err := gofeed.NewParser().Parse(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return Feed{}, err
	}
	f := Feed{Title: parsed.Title, Description: parsed.Description}
	if parsed.Image != nil {
		f.ImageURL = parsed.Image.URL
	}
	if parsed.ITunesExt != nil && parsed.ITunesExt.Image != "" {
		f.ImageURL = parsed.ITunesExt.Image
	}
	for _, it := range parsed.Items {
		ep := Episode{Title: it.Title, Description: it.Description, PubDate: it.PublishedParsed}
		if len(it.Enclosures) > 0 {
			ep.AudioURL = it.Enclosures[0].URL
			ep.ContentType = it.Enclosures[0].Type
		}
		if ep.AudioURL == "" {
			continue // skip items with no audio
		}
		f.Episodes = append(f.Episodes, ep)
	}
	return f, nil
}

// DownloadTo validates the URL then downloads it to destPath.
func DownloadTo(ctx context.Context, srcURL, destPath string) (size int64, contentType, suffix string, err error) {
	if err := ValidateURL(srcURL); err != nil {
		return 0, "", "", err
	}
	return DownloadToUnsafe(ctx, guardedClient(), srcURL, destPath)
}

// DownloadToUnsafe downloads srcURL to destPath using the given client WITHOUT the SSRF guard.
// WARNING: for tests only — callers are responsible for ensuring the URL is safe.
func DownloadToUnsafe(ctx context.Context, client *http.Client, srcURL, destPath string) (size int64, contentType, suffix string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return 0, "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("podcast: download %s: status %d", srcURL, resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, "", "", err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return 0, "", "", err
	}
	defer out.Close()
	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return 0, "", "", err
	}
	contentType = resp.Header.Get("Content-Type")
	// suffix is derived from destPath's extension (caller-chosen), not the remote resource.
	suffix = strings.TrimPrefix(filepath.Ext(destPath), ".")
	return n, contentType, suffix, nil
}
