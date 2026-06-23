// Package carddav is an HTTP CardDAV server built on top of dav.Service (implements carddav.Backend
// from emersion/go-webdav). Round-trip fidelity: the raw PUT body is stored as the source of truth.
package carddav

import "strings"

const prefix = "/carddav"

// homeSeg is the fixed home-set path segment. go-webdav determines the resource type by
// PATH DEPTH: principal=1 segment, addressbook-home-set=2, addressbook=3, object=4.
// That is why principal (/carddav/{uid}/) and home-set (/carddav/{uid}/card/) live at different depths.
const homeSeg = "card"

func principalPath(userID string) string { return prefix + "/" + userID + "/" }
func homeSetPath(userID string) string   { return prefix + "/" + userID + "/" + homeSeg + "/" }
func addressbookPath(userID, uri string) string {
	return prefix + "/" + userID + "/" + homeSeg + "/" + uri + "/"
}
func objectPath(userID, uri, uid string) string {
	return prefix + "/" + userID + "/" + homeSeg + "/" + uri + "/" + uid + ".vcf"
}

// parsePath parses a CardDAV path into (userID, uri, objName). Tolerant of leading
// prefix and trailing slash. Forms (after the prefix):
//
//	/{uid}/                      → (uid, "", "")          principal
//	/{uid}/card/                 → (uid, "", "")          home-set
//	/{uid}/card/{uri}/           → (uid, uri, "")          addressbook
//	/{uid}/card/{uri}/{obj}.vcf  → (uid, uri, obj)         object
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
	// parts[1] is the fixed homeSeg ("card"), skip it
	if len(parts) >= 3 {
		uri = parts[2]
	}
	if len(parts) >= 4 {
		objName = strings.TrimSuffix(parts[3], ".vcf")
	}
	return userID, uri, objName
}
