package api

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a fixed-window per-key attempt counter (login brute-force protection).
// In-memory for our single binary; the map is reset at the start of each window
// so it does not grow without bound.
type rateLimiter struct {
	mu          sync.Mutex
	counts      map[string]int
	limit       int
	window      time.Duration
	windowStart time.Time
}

// newLoginLimiter allows at most limit attempts per IP per minute.
func newLoginLimiter() *rateLimiter {
	return &rateLimiter{counts: make(map[string]int), limit: 10, window: time.Minute, windowStart: time.Now()}
}

// allow records an attempt and reports whether the key is within the limit.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if time.Since(rl.windowStart) > rl.window {
		rl.counts = make(map[string]int)
		rl.windowStart = time.Now()
	}
	if rl.counts[key] >= rl.limit {
		return false
	}
	rl.counts[key]++
	return true
}

// rateLimited wraps a handler, rejecting requests that exceed the per-IP limit.
func (s *Server) rateLimited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.loginLimiter.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "too many attempts, please try again later")
			return
		}
		h(w, r)
	}
}

// newPollLimiter is the ceiling for /pair/token polling (the daemon polls frequently).
func newPollLimiter() *rateLimiter {
	return &rateLimiter{counts: make(map[string]int), limit: 120, window: time.Minute, windowStart: time.Now()}
}

// pollLimited is like rateLimited but uses the more generous poll-specific limiter.
func (s *Server) pollLimited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.pollLimiter.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "too many requests, please try again later")
			return
		}
		h(w, r)
	}
}
