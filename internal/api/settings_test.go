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
