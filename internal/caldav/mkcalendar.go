package caldav

import (
	"encoding/xml"
	"net/http"
	"strings"
)

// MKCALENDAR (RFC 4791) is not dispatched by go-webdav at all → 405. Apple Reminders (remindd)
// uses this method to create a task list (VTODO collection). We intercept it in middleware and
// create the collection at the client-chosen path (its segment = uri), with the component-set
// from the request body (VTODO for task lists, VEVENT for calendars). The body may be absent.

type mkcalReq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav mkcalendar"`
	Set     struct {
		Prop struct {
			DisplayName string `xml:"DAV: displayname"`
			CompSet     struct {
				Comps []struct {
					Name string `xml:"name,attr"`
				} `xml:"urn:ietf:params:xml:ns:caldav comp"`
			} `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
		} `xml:"DAV: prop"`
	} `xml:"DAV: set"`
}

// HandleMkcalendar creates a collection as requested by the client (MKCALENDAR). Returns 201 on success.
func (b *Backend) HandleMkcalendar(w http.ResponseWriter, r *http.Request) {
	uid := userID(r.Context())
	_, uri, _ := parsePath(r.URL.Path)
	if uri == "" {
		http.Error(w, "caldav: invalid collection path", http.StatusBadRequest)
		return
	}

	var req mkcalReq
	_ = xml.NewDecoder(r.Body).Decode(&req) // empty/malformed body — use defaults

	name := strings.TrimSpace(req.Set.Prop.DisplayName)
	// Apple sends an unresolved placeholder when auto-creating the default list.
	if name == "" || name == "DEFAULT_TASK_CALENDAR_NAME" {
		name = "Reminders"
	}
	var comps []string
	for _, c := range req.Set.Prop.CompSet.Comps {
		if n := strings.TrimSpace(c.Name); n != "" {
			comps = append(comps, n)
		}
	}
	components := "VEVENT"
	if len(comps) > 0 {
		components = strings.Join(comps, ",")
	}

	if _, err := b.svc.CreateCalendarWithURI(r.Context(), uid, uri, name, components); err != nil {
		http.Error(w, "caldav: failed to create collection", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
