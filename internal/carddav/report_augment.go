package carddav

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/beevik/etree"

	"discodrive/internal/db"
)

// REPORT (addressbook-multiget/query) in go-webdav returns a RE-SERIALIZED vCard (decode→encode
// via go-vcard), which corrupts data (photos → truncated data-URIs, reordering of parameters)
// and accumulates corruption with each round of edits. We serve the raw vCard byte-for-byte here too.
// Technique: etree holds the multistatus structure; address-data is replaced with a placeholder,
// and after serialization the raw vCard is substituted back with manual XML escaping
// (CR → &#13; so that CRLF survives XML-parser normalization on the client).

func (b *Backend) serveReport(w http.ResponseWriter, r *http.Request, dav http.Handler) {
	buf := newBufRW()
	dav.ServeHTTP(buf, r)
	body := []byte(buf.body.String())
	if buf.status == http.StatusMultiStatus {
		body = b.augmentReport(r.Context(), body)
	}
	for k, v := range buf.header {
		w.Header()[k] = v
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(buf.status)
	_, _ = w.Write(body)
}

var vcardXMLEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", "&#13;")

func (b *Backend) augmentReport(ctx context.Context, body []byte) []byte {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(body); err != nil {
		return body
	}
	ms := doc.Root()
	if ms == nil || ms.Tag != "multistatus" {
		return body
	}
	repl := map[string]string{}
	n := 0
	for _, resp := range ms.SelectElements("response") {
		href := resp.SelectElement("href")
		if href == nil {
			continue
		}
		_, uri, obj := parsePath(href.Text())
		if uri == "" || obj == "" {
			continue
		}
		ad := resp.FindElement(".//address-data")
		if ad == nil {
			continue
		}
		ab, err := b.resolveAddressbook(ctx, uri)
		if err != nil {
			continue
		}
		data, _, err := b.svc.GetAddressbookObject(ctx, db.UUIDString(ab.ID), obj)
		if err != nil {
			continue
		}
		ph := fmt.Sprintf("__KF_RAWVCARD_%d__", n)
		n++
		ad.SetText(ph)
		repl[ph] = vcardXMLEscaper.Replace(data)
	}
	if n == 0 {
		return body
	}
	out, err := doc.WriteToString()
	if err != nil {
		return body
	}
	for ph, esc := range repl {
		out = strings.Replace(out, ph, esc, 1)
	}
	return []byte(out)
}
