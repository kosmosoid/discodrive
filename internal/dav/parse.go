package dav

import (
	"encoding/json"
	"strings"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
)

// parseICal extracts the uid and cache fields of an event from raw iCalendar data.
// Resilient: on error returns an empty uid and a valid JSON "{}".
func parseICal(data string) (uid string, parsed []byte) {
	empty := []byte("{}")
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return "", empty
	}
	m := map[string]any{}
	for _, comp := range cal.Children {
		if comp.Name != ical.CompEvent && comp.Name != ical.CompToDo {
			continue
		}
		m["component"] = comp.Name
		if p := comp.Props.Get(ical.PropUID); p != nil {
			uid = p.Value
			m["uid"] = p.Value
		}
		if p := comp.Props.Get(ical.PropSummary); p != nil {
			m["summary"] = p.Value
		}
		if p := comp.Props.Get(ical.PropDateTimeStart); p != nil {
			m["dtstart"] = p.Value
			m["all_day"] = p.Params.Get(ical.ParamValue) == "DATE"
		}
		if p := comp.Props.Get(ical.PropDateTimeEnd); p != nil {
			m["dtend"] = p.Value
		}
		if p := comp.Props.Get(ical.PropRecurrenceRule); p != nil {
			m["rrule"] = p.Value
		}
		break
	}
	b, err := json.Marshal(m)
	if err != nil {
		return uid, empty
	}
	return uid, b
}

// parseVCard extracts the uid and cache fields of a contact from raw vCard data.
func parseVCard(data string) (uid string, parsed []byte) {
	empty := []byte("{}")
	card, err := vcard.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return "", empty
	}
	m := map[string]any{}
	if fn := card.Value(vcard.FieldFormattedName); fn != "" {
		m["full_name"] = fn
	}
	if emails := card.Values(vcard.FieldEmail); len(emails) > 0 {
		m["emails"] = emails
	}
	if tels := card.Values(vcard.FieldTelephone); len(tels) > 0 {
		m["phones"] = tels
	}
	if u := card.Value(vcard.FieldUID); u != "" {
		uid = u
		m["uid"] = u
	}
	b, err := json.Marshal(m)
	if err != nil {
		return uid, empty
	}
	return uid, b
}
