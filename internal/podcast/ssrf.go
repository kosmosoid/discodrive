package podcast

import (
	"time"

	"discodrive/internal/fetchguard"
)

// The SSRF guard lives in internal/fetchguard (shared with saved items); these
// aliases keep the podcast package's call sites and public names unchanged.

// ErrBlockedURL is returned for URLs that fail the SSRF guard.
var ErrBlockedURL = fetchguard.ErrBlocked

// ValidateURL enforces http/https and rejects hosts that resolve to loopback,
// private, or link-local addresses.
func ValidateURL(raw string) error { return fetchguard.ValidateURL(raw) }

// safeDialer pins the SSRF check to the actually dialed IP (DNS-rebinding safe).
var safeDialer = fetchguard.Dialer

// httpTimeout bounds all outbound podcast fetches.
const httpTimeout = 30 * time.Second
