package dav

import (
	"encoding/json"
	"testing"
)

const sampleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\nUID:evt-1@test\r\nSUMMARY:Обед\r\nDTSTART:20260612T120000Z\r\nDTEND:20260612T130000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

const sampleVCF = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Иван Петров\r\nEMAIL:ivan@example.com\r\nTEL:+79991234567\r\nUID:card-1\r\nEND:VCARD\r\n"

func TestParseICalExtractsFields(t *testing.T) {
	uid, parsed := parseICal(sampleICS)
	if uid != "evt-1@test" {
		t.Fatalf("uid = %q, expected evt-1@test", uid)
	}
	var m map[string]any
	if err := json.Unmarshal(parsed, &m); err != nil {
		t.Fatalf("parsed is not JSON: %v", err)
	}
	if m["summary"] != "Обед" {
		t.Fatalf("summary = %v, expected Обед", m["summary"])
	}
	if m["component"] != "VEVENT" {
		t.Fatalf("component = %v, expected VEVENT", m["component"])
	}
}

func TestParseVCardExtractsFields(t *testing.T) {
	uid, parsed := parseVCard(sampleVCF)
	if uid != "card-1" {
		t.Fatalf("uid = %q, expected card-1", uid)
	}
	var m map[string]any
	if err := json.Unmarshal(parsed, &m); err != nil {
		t.Fatalf("parsed is not JSON: %v", err)
	}
	if m["full_name"] != "Иван Петров" {
		t.Fatalf("full_name = %v", m["full_name"])
	}
	emails, _ := m["emails"].([]any)
	if len(emails) != 1 || emails[0] != "ivan@example.com" {
		t.Fatalf("emails = %v", m["emails"])
	}
}

func TestParseBrokenStillReturnsJSON(t *testing.T) {
	uid, parsed := parseICal("this is not iCalendar")
	_ = uid
	var m map[string]any
	if err := json.Unmarshal(parsed, &m); err != nil {
		t.Fatalf("parsed broken object must be valid JSON, got %q (%v)", parsed, err)
	}
}
