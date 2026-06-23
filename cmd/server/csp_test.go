package main

import (
	"strings"
	"testing"

	"discodrive"
)

// The embedded SPA currently ships two executable inline scripts (anti-FOUC theme
// toggle + Nuxt bootstrap). inlineScriptHashes must extract both so the CSP can drop
// 'unsafe-inline'. If this count changes after a web rebuild, the CSP simply picks up
// the new hashes — the assertion just guards against extraction silently breaking
// (which would fall back to 'unsafe-inline').
func TestInlineScriptHashes(t *testing.T) {
	got := inlineScriptHashes(discodrive.WebUI())
	if len(got) != 2 {
		t.Fatalf("expected 2 inline-script hashes from the embedded SPA, got %d: %v", len(got), got)
	}
	for _, h := range got {
		if !strings.HasPrefix(h, "'sha256-") {
			t.Errorf("hash %q is not a sha256 source expression", h)
		}
	}
}
