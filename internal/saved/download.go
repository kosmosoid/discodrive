package saved

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

// errTooLarge aborts a download that exceeds the configured size limit.
// The message reaches the user (notification + web UI), so it names the actual
// numbers and the knob to turn — no bare "size limit".
var errTooLarge = errors.New("download exceeds size limit")

func sizeLimitErr(size, max int64) error {
	if size > 0 {
		return fmt.Errorf("%w: file is %s, limit is %s (SAVED_MAX_DOWNLOAD_MB)", errTooLarge, humanBytes(size), humanBytes(max))
	}
	return fmt.Errorf("%w: limit is %s (SAVED_MAX_DOWNLOAD_MB)", errTooLarge, humanBytes(max))
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// processDownload streams the URL to <user>/downloads/<name> via a staging
// file in the root-level .tmp/ (outside user trees, so fsnotify/rescan never
// see a partial file); the final rename makes the file appear atomically.
func (s *Service) processDownload(ctx context.Context, item db.SavedItem) (result, error) {
	if err := s.Validate(item.Url); err != nil {
		return result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.Url, nil)
	if err != nil {
		return result{}, err
	}
	// The browser session cookie, for sites behind a login. net/http strips
	// Cookie itself on a cross-domain redirect, so the session cannot leak to
	// a foreign host.
	if item.CookieHeader.Valid && item.CookieHeader.String != "" {
		req.Header.Set("Cookie", item.CookieHeader.String)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	var total pgtype.Int8
	if resp.ContentLength > 0 {
		if s.maxDownload > 0 && resp.ContentLength > s.maxDownload {
			return result{}, sizeLimitErr(resp.ContentLength, s.maxDownload)
		}
		total = pgtype.Int8{Int64: resp.ContentLength, Valid: true}
	}

	name := downloadFilename(resp.Header.Get("Content-Disposition"), item.Url)
	uid := db.UUIDString(item.UserID)
	destRel, err := s.freeName(uid+"/Downloads", name)
	if err != nil {
		return result{}, err
	}

	pr := &progressReader{ctx: ctx, r: resp.Body, q: s.q, id: item.ID, max: s.maxDownload, total: total}
	tmpRel := ".tmp/saved-" + randHex(16)
	size, _, err := s.st.WriteFile(tmpRel, pr)
	if err != nil {
		_ = s.st.Remove(tmpRel)
		if pr.err != nil {
			err = pr.err // the reader's verdict (deleted/too large) beats io.Copy's wrapper
		}
		return result{}, err
	}
	if err := s.st.Move(tmpRel, destRel); err != nil {
		_ = s.st.Remove(tmpRel)
		return result{}, err
	}

	res := result{
		contentPath: pgtype.Text{String: destRel, Valid: true},
		size:        pgtype.Int8{Int64: size, Valid: true},
	}
	if item.Title == "" {
		res.title = name
	}
	return res, nil
}

// freeName returns "<dir>/<name>", appending -2, -3, … before the extension
// while the name is taken on disk.
func (s *Service) freeName(dir, name string) (string, error) {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		rel := dir + "/" + candidate
		abs, err := s.st.AbsPath(rel)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
			return rel, nil
		}
	}
}

// downloadFilename derives a safe file name from the Content-Disposition
// header, falling back to the URL path, then to "download".
func downloadFilename(disposition, rawURL string) string {
	var name string
	if disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			name = params["filename"]
		}
	}
	if name == "" {
		if u, err := url.Parse(rawURL); err == nil {
			if base := path.Base(u.Path); base != "/" && base != "." {
				if dec, err := url.PathUnescape(base); err == nil {
					base = dec
				}
				name = base
			}
		}
	}
	return sanitizeFilename(name)
}

// sanitizeFilename neutralizes path tricks in an attacker-controlled name
// (e.g. "filename=../../x" in Content-Disposition): path separators and NUL
// become "_", the basename is taken, leading dots/spaces are trimmed (no
// hidden files), and the result is capped preserving the extension.
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', 0:
			return '_'
		}
		return r
	}, name)
	name = path.Base("/" + name)
	if name == "/" || name == "." {
		name = ""
	}
	name = strings.TrimLeft(name, ". ")
	name = strings.TrimRight(name, ". ")
	const maxName = 150
	if len(name) > maxName {
		ext := path.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		base := strings.TrimSuffix(name, ext)
		for len(base)+len(ext) > maxName || !isUTF8Boundary(base) {
			base = base[:len(base)-1]
		}
		name = base + ext
	}
	if name == "" {
		name = "download"
	}
	return name
}

// isUTF8Boundary reports whether s does not end mid-rune.
func isUTF8Boundary(s string) bool {
	if s == "" {
		return true
	}
	return strings.ToValidUTF8(s, "") == s
}

// progressReader counts bytes, enforces the size limit, and persists progress
// at most once per second. A progress UPDATE matching zero rows means the item
// was deleted mid-download — the reader aborts with errDeleted so the caller
// discards the staging file.
type progressReader struct {
	ctx   context.Context
	r     io.Reader
	q     *db.Queries
	id    pgtype.UUID
	max   int64
	total pgtype.Int8

	read int64
	last time.Time
	err  error // sticky verdict: errDeleted or errTooLarge
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.max > 0 && p.read > p.max {
		// The size is unknown up front (chunked), so report the limit itself.
		p.err = sizeLimitErr(p.total.Int64, p.max)
		return n, p.err
	}
	if time.Since(p.last) >= time.Second {
		p.last = time.Now()
		rows, uerr := p.q.UpdateSavedItemProgress(p.ctx, db.UpdateSavedItemProgressParams{
			ID:        p.id,
			BytesDone: p.read,
			SizeBytes: p.total,
		})
		if uerr == nil && rows == 0 {
			p.err = errDeleted
			return n, errDeleted
		}
	}
	return n, err
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
