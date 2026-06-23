// Package dav stores CalDAV/CardDAV objects: raw data is the source of truth (round-trip),
// parsed jsonb is a cache, etag (hash) + ctag (collection counter). HTTP-DAV is layer 2.2/2.3.
package dav

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

// slugAlphabet — lowercase letters + digits (URL-safe, readable in /dav/c/{uri}/).
const slugAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// newFeedToken returns a long unguessable token for public feed subscription links.
func newFeedToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return newSlug() + newSlug()
	}
	return hex.EncodeToString(b)
}

// newSlug returns a random 10-character slug for collection URLs.
func newSlug() string {
	b := make([]byte, 10)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(slugAlphabet))))
		if err != nil {
			b[i] = slugAlphabet[0]
			continue
		}
		b[i] = slugAlphabet[n.Int64()]
	}
	return string(b)
}
