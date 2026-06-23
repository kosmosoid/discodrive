package podcast

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"time"
)

// ErrBlockedURL is returned for URLs that fail the SSRF guard.
var ErrBlockedURL = errors.New("podcast: blocked URL")

// ValidateURL enforces http/https and rejects hosts that resolve to loopback,
// private, or link-local addresses. It is a best-effort SSRF guard for a
// personal server; it does not defend against DNS rebinding.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: parse: %v", ErrBlockedURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q", ErrBlockedURL, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrBlockedURL)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Can't resolve (offline/unknown). Reject to be safe.
		return fmt.Errorf("%w: resolve %q: %v", ErrBlockedURL, host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("%w: %s -> %s", ErrBlockedURL, host, ip)
		}
	}
	return nil
}

// isBlockedIP reports whether ip is loopback, private, link-local, or unspecified.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// safeDialer re-checks the *resolved* IP at connection time (after DNS, before
// connect) and rejects blocked targets. ValidateURL alone is a TOCTOU gate: it
// resolves the host once, but http.Client re-resolves at dial time, so a DNS
// rebinding attacker can pass validation with a public IP then serve a private
// IP for the real request. Pinning the check to the actual dialed address closes
// that gap — it is the authoritative SSRF guard; ValidateURL is the cheap first pass.
var safeDialer = &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 30 * time.Second,
	Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: dial address %q: %v", ErrBlockedURL, address, err)
		}
		ip := net.ParseIP(host)
		if ip == nil || isBlockedIP(ip) {
			return fmt.Errorf("%w: dial %s", ErrBlockedURL, address)
		}
		return nil
	},
}

// httpTimeout bounds all outbound podcast fetches.
const httpTimeout = 30 * time.Second
