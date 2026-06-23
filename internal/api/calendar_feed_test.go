package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

func dbUUID(t *testing.T, c db.Calendar) string { t.Helper(); return db.UUIDString(c.ID) }

func TestCalendarFeedPublic(t *testing.T) {
	svc, ownerID, ctx := setupCalendars(t) // helper from calendars_test.go
	cal, _ := svc.CreateCalendar(ctx, ownerID, "Фид", "")
	calID := dbUUID(t, cal)
	// put an event
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//EN\r\nBEGIN:VEVENT\r\nUID:fe1\r\nDTSTAMP:20260611T000000Z\r\nDTSTART:20260620T100000Z\r\nDTEND:20260620T110000Z\r\nSUMMARY:Публичная встреча\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := svc.PutCalendarObject(ctx, calID, "fe1", raw); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// feed link without a password
	tok, err := svc.CreateCalendarFeedLink(ctx, ownerID, calID, "")
	if err != nil {
		t.Fatalf("feed link: %v", err)
	}
	s := &Server{dav: svc, feedLimiter: newLoginLimiter()}

	// GET without password → 200 ICS with the event
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cal/"+tok+".ics", nil)
	req.SetPathValue("file", tok+".ics")
	s.handleCalendarFeed(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Публичная встреча") {
		t.Fatalf("expected 200 with the event, got %d:\n%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Fatalf("Content-Type=%q", ct)
	}

	// feed link with a password
	hash, _ := auth.HashPassword("secret")
	tokp, _ := svc.CreateCalendarFeedLink(ctx, ownerID, calID, hash)
	// no auth → 401
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/cal/"+tokp+".ics", nil)
	req2.SetPathValue("file", tokp+".ics")
	s.handleCalendarFeed(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a password, got %d", rec2.Code)
	}
	// correct password → 200
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "/cal/"+tokp+".ics", nil)
	req3.SetPathValue("file", tokp+".ics")
	req3.SetBasicAuth("x", "secret")
	s.handleCalendarFeed(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 with a password, got %d", rec3.Code)
	}

	// unknown token → 404
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("GET", "/cal/nope.ics", nil)
	req4.SetPathValue("file", "nope.ics")
	s.handleCalendarFeed(rec4, req4)
	if rec4.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown token, got %d", rec4.Code)
	}
}
