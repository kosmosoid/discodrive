package opds

import "net/url"

// encodeBookID returns the book UUID as-is for use in URL path segments.
func encodeBookID(id string) string { return id }

// decodeBookID returns the book ID as-is (UUIDs need no unescaping).
func decodeBookID(id string) string { return id }

// encodeNameID percent-encodes a name (author/series/genre) for use in a URL path segment.
func encodeNameID(name string) string { return url.PathEscape(name) }

// decodeNameID decodes a percent-encoded name from a URL path segment.
// Returns the original id if unescaping fails.
func decodeNameID(id string) string {
	s, err := url.PathUnescape(id)
	if err != nil {
		return id
	}
	return s
}
