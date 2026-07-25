package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// authLimited: successful requests never consume the budget (bulk dd-cli/dd-mcp
// runs each exchange a device token and must not hit 429), failures still do.
func TestAuthLimitedSuccessesNotCounted(t *testing.T) {
	s := &Server{loginLimiter: newLoginLimiter()}
	ok := s.authLimited(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		ok(rec, httptest.NewRequest(http.MethodPost, "/auth/device/token", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, successful auth must never be throttled", i, rec.Code)
		}
	}
}

func TestAuthLimitedFailuresStillLimited(t *testing.T) {
	s := &Server{loginLimiter: newLoginLimiter()}
	fail := s.authLimited(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusUnauthorized, "device not authorized")
	})
	var got429 bool
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		fail(rec, httptest.NewRequest(http.MethodPost, "/auth/device/token", nil))
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: got %d, want 401 or 429", i, rec.Code)
		}
	}
	if !got429 {
		t.Fatal("repeated failures must eventually get 429 (brute-force protection)")
	}
}

// A failure burst must not lock out a subsequently valid client forever within the
// window — but within the same window the budget is shared per IP, so after the
// limit is consumed even successes are rejected until the window rolls. This
// documents the trade-off deliberately.
func TestAuthLimitedFailureBudgetBlocksWindow(t *testing.T) {
	s := &Server{loginLimiter: newLoginLimiter()}
	fail := s.authLimited(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusUnauthorized, "nope")
	})
	for i := 0; i < 10; i++ {
		fail(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/auth/device/token", nil))
	}
	rec := httptest.NewRecorder()
	fail(rec, httptest.NewRequest(http.MethodPost, "/auth/device/token", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after exhausting the failure budget: got %d, want 429", rec.Code)
	}
}
