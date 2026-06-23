package dav

import (
	"regexp"
	"testing"
)

func TestNewSlugFormat(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-z]{10}$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		s := newSlug()
		if !re.MatchString(s) {
			t.Fatalf("slug %q does not match the format [0-9a-z]{10}", s)
		}
		if seen[s] {
			t.Fatalf("slug collision at iteration %d: %q", i, s)
		}
		seen[s] = true
	}
}
