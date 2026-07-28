package bookmarks

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	maxPageBytes    = 2 << 20 // HTML page fetch cap (title/favicon extraction)
	maxFaviconBytes = 512 << 10
)

// faviconRel is the storage-root-relative favicon path — outside the user
// tree (like podcast covers), so rescan/sync never see these files.
func faviconRel(uid, id, ext string) string {
	return "saved/" + uid + "/favicons/" + id + ext
}

// fetchPageMeta downloads the page (capped) and returns its <title> and the
// href of the best <link rel=icon>. Non-HTML or unparsable pages yield empty values.
func (s *Service) fetchPageMeta(ctx context.Context, rawURL string) (title, iconHref string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body := io.LimitReader(resp.Body, maxPageBytes)
	// Decode legacy charsets (windows-1251 & co) to UTF-8 for the tokenizer.
	decoded, err := charset.NewReader(body, resp.Header.Get("Content-Type"))
	if err != nil {
		decoded = body
	}

	z := html.NewTokenizer(decoded)
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(title), iconHref, nil // EOF or cap reached: keep what we have
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			switch tok.Data {
			case "title":
				if title == "" && z.Next() == html.TextToken {
					title = z.Token().Data
				}
			case "link":
				var rel, href string
				for _, a := range tok.Attr {
					switch a.Key {
					case "rel":
						rel = strings.ToLower(a.Val)
					case "href":
						href = a.Val
					}
				}
				// "icon", "shortcut icon", "apple-touch-icon" — first match wins.
				if iconHref == "" && href != "" && strings.Contains(rel, "icon") {
					iconHref = href
				}
			case "body":
				// <title>/<link> live in <head>; stop early on huge pages.
				return strings.TrimSpace(title), iconHref, nil
			}
		}
	}
}

// fetchFavicon downloads the page's icon (or /favicon.ico as fallback) into
// faviconRel(uid, id, ext) and returns the extension ("" on any failure).
func (s *Service) fetchFavicon(ctx context.Context, uid, id string, pageURL *url.URL, iconHref string) string {
	candidates := []string{}
	if iconHref != "" {
		if u, err := pageURL.Parse(iconHref); err == nil {
			candidates = append(candidates, u.String())
		}
	}
	candidates = append(candidates, pageURL.Scheme+"://"+pageURL.Host+"/favicon.ico")

	for _, cand := range candidates {
		if err := s.Validate(cand); err != nil {
			continue
		}
		ext, err := s.downloadFavicon(ctx, uid, id, cand)
		if err == nil {
			return ext
		}
		log.Printf("discodrive: bookmark favicon %s: %v", cand, err)
	}
	return ""
}

func (s *Service) downloadFavicon(ctx context.Context, uid, id, iconURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	ext := faviconExt(resp.Header.Get("Content-Type"), iconURL)
	if ext == "" {
		return "", fmt.Errorf("not an image: %q", resp.Header.Get("Content-Type"))
	}
	rel := faviconRel(uid, id, ext)
	n, _, err := s.st.WriteFile(rel, io.LimitReader(resp.Body, maxFaviconBytes))
	if err != nil {
		return "", err
	}
	if n == 0 {
		_ = s.st.Remove(rel)
		return "", fmt.Errorf("empty favicon body")
	}
	return ext, nil
}

// faviconExt maps the response content type (or the URL extension) to a file
// extension; "" means the response is not a supported image.
func faviconExt(contentType, iconURL string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		switch mt {
		case "image/x-icon", "image/vnd.microsoft.icon":
			return ".ico"
		case "image/png":
			return ".png"
		case "image/svg+xml":
			return ".svg"
		case "image/jpeg":
			return ".jpg"
		case "image/gif":
			return ".gif"
		case "image/webp":
			return ".webp"
		}
	}
	if u, err := url.Parse(iconURL); err == nil {
		switch strings.ToLower(path.Ext(u.Path)) {
		case ".ico", ".png", ".svg", ".jpg", ".jpeg", ".gif", ".webp":
			return strings.ToLower(path.Ext(u.Path))
		}
	}
	return ""
}
