package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func putLang(h http.Handler, bearer, lang string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"language": lang})
	req := httptest.NewRequest(http.MethodPut, "/me/language", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Default language is en; changes persist to the DB; invalid language codes are rejected.
func TestUserLanguageGetSetDefault(t *testing.T) {
	ctx := context.Background()
	_, q, svc := bootstrapPairingDB(t)
	userTok, _, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	s := &Server{auth: svc, q: q}
	getH := svc.Middleware(http.HandlerFunc(s.handleGetLanguage))
	setH := svc.Middleware(http.HandlerFunc(s.handleSetLanguage))

	if rec, m := doGet(getH, "/me/language", userTok); rec.Code != http.StatusOK || m["language"] != "en" {
		t.Fatalf("default en: code=%d body=%v", rec.Code, m)
	}
	if rec := putLang(setH, userTok, "ru"); rec.Code != http.StatusOK {
		t.Fatalf("set ru: code=%d", rec.Code)
	}
	if _, m := doGet(getH, "/me/language", userTok); m["language"] != "ru" {
		t.Fatalf("after set ru: %v", m)
	}
	if rec := putLang(setH, userTok, "xx"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid language → expected 400, got %d", rec.Code)
	}
}

func putSession(h http.Handler, bearer string, ttl any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]any{"ttl_minutes": ttl})
	req := httptest.NewRequest(http.MethodPut, "/me/session", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The session lifetime defaults to an hour, accepts 0 ("never sign me out") and the
// preset the UI offers, and refuses values that would make the session unusable.
func TestUserSessionTTLGetSetDefault(t *testing.T) {
	ctx := context.Background()
	_, q, svc := bootstrapPairingDB(t)
	userTok, _, err := svc.Register(ctx, "u@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	s := &Server{auth: svc, q: q}
	getH := svc.Middleware(http.HandlerFunc(s.handleGetSession))
	setH := svc.Middleware(http.HandlerFunc(s.handleSetSession))

	if rec, m := doGet(getH, "/me/session", userTok); rec.Code != http.StatusOK || m["ttl_minutes"] != float64(60) {
		t.Fatalf("default 60: code=%d body=%v", rec.Code, m)
	}
	if rec := putSession(setH, userTok, 1440); rec.Code != http.StatusOK {
		t.Fatalf("set 1440: code=%d", rec.Code)
	}
	if _, m := doGet(getH, "/me/session", userTok); m["ttl_minutes"] != float64(1440) {
		t.Fatalf("after set 1440: %v", m)
	}
	if rec := putSession(setH, userTok, 0); rec.Code != http.StatusOK {
		t.Fatalf("set 0 (never): code=%d", rec.Code)
	}
	if _, m := doGet(getH, "/me/session", userTok); m["ttl_minutes"] != float64(0) {
		t.Fatalf("after set 0: %v", m)
	}
	// A minute-long session logs you out mid-task; a negative one is nonsense.
	for _, bad := range []any{1, -60, 600000} {
		if rec := putSession(setH, userTok, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("ttl_minutes=%v → expected 400, got %d", bad, rec.Code)
		}
	}
	// Missing field is a bad request, not a silent reset to zero.
	req := httptest.NewRequest(http.MethodPut, "/me/session", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+userTok)
	rec := httptest.NewRecorder()
	setH.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body → expected 400, got %d", rec.Code)
	}
}
