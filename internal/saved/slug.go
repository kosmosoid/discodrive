package saved

import (
	"strings"
	"unicode"
)

// maxSlugRunes caps article slugs; combined with "<year>/" and ".md" the file
// name stays well within filesystem limits.
const maxSlugRunes = 80

// Slugify turns a title into a file-name slug: lower-cased, unicode letters
// and digits kept, everything else collapsed into single dashes. Returns ""
// when nothing survives (caller falls back to the site host).
func Slugify(title string) string {
	var b strings.Builder
	dash := true // suppress a leading dash
	count := 0
	for _, r := range strings.ToLower(title) {
		if count >= maxSlugRunes {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			count++
		} else if !dash {
			b.WriteRune('-')
			dash = true
			count++
		}
	}
	return strings.Trim(b.String(), "-")
}
