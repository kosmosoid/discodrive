package carddav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"discodrive/internal/db"
)

type fakeSettings struct{ enabled bool }

func (f fakeSettings) GetSetting(_ context.Context, key string) (db.Setting, error) {
	if key == "carddav.enabled" && f.enabled {
		return db.Setting{Key: key, Value: "true"}, nil
	}
	return db.Setting{Value: "false"}, nil
}

func TestHandlerForbiddenWhenDisabled(t *testing.T) {
	h := Handler(nil, fakeSettings{enabled: false}, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("PROPFIND", "/carddav/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestWellKnownRedirects(t *testing.T) {
	rec := httptest.NewRecorder()
	WellKnown().ServeHTTP(rec, httptest.NewRequest("PROPFIND", "/.well-known/carddav", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/carddav/" {
		t.Fatalf("well-known: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}
