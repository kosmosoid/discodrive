// Package caldav is an HTTP CalDAV server built on top of dav.Service (implements caldav.Backend
// from emersion/go-webdav). Round-trip fidelity: the raw PUT body is stored as the source of truth.
package caldav

import "strings"

const prefix = "/caldav"

// homeSeg is the fixed home-set path segment. go-webdav determines the resource type by
// PATH DEPTH: principal=1 segment, calendar-home-set=2, calendar=3, object=4.
// That is why principal (/caldav/{uid}/) and home-set (/caldav/{uid}/cal/) live at different depths.
const homeSeg = "cal"

func principalPath(userID string) string { return prefix + "/" + userID + "/" }
func homeSetPath(userID string) string   { return prefix + "/" + userID + "/" + homeSeg + "/" }
func calendarPath(userID, uri string) string {
	return prefix + "/" + userID + "/" + homeSeg + "/" + uri + "/"
}
func objectPath(userID, uri, uid string) string {
	return prefix + "/" + userID + "/" + homeSeg + "/" + uri + "/" + uid + ".ics"
}

// parsePath parses a CalDAV path into (userID, uri, objName). Tolerant of leading
// prefix and trailing slash. Forms (after the prefix):
//
//	/{uid}/                      → (uid, "", "")          principal
//	/{uid}/cal/                  → (uid, "", "")          home-set
//	/{uid}/cal/{uri}/            → (uid, uri, "")          calendar
//	/{uid}/cal/{uri}/{obj}.ics   → (uid, uri, obj)         object
func parsePath(path string) (userID, uri, objName string) {
	p := strings.TrimPrefix(path, prefix)
	p = strings.Trim(p, "/")
	if p == "" {
		return "", "", ""
	}
	parts := strings.Split(p, "/")
	if len(parts) >= 1 {
		userID = parts[0]
	}
	// parts[1] is the fixed homeSeg ("cal"), skip it
	if len(parts) >= 3 {
		uri = parts[2]
	}
	if len(parts) >= 4 {
		objName = strings.TrimSuffix(parts[3], ".ics")
	}
	return userID, uri, objName
}
