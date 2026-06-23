package subsonic

import "strings"

// encID encodes a UUID with a type prefix to create an opaque Subsonic id.
// kind must be one of "ar" (artist), "al" (album), "tr" (track), "pl" (playlist).
// The result is "kind-uuid", e.g. "ar-550e8400-e29b-41d4-a716-446655440000".
func encID(kind, uuid string) string {
	return kind + "-" + uuid
}

// decID decodes an opaque Subsonic id back into its kind and UUID.
// It splits on the first '-' separator (not the UUID's own dashes).
// ok is false if the id is empty or otherwise malformed (missing separator or empty parts).
func decID(id string) (kind, uuid string, ok bool) {
	idx := strings.Index(id, "-")
	if idx <= 0 || idx == len(id)-1 {
		return "", "", false
	}
	kind = id[:idx]
	uuid = id[idx+1:]
	if uuid == "" {
		return "", "", false
	}
	return kind, uuid, true
}
