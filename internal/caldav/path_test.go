package caldav

import "testing"

func TestParsePath(t *testing.T) {
	cases := []struct {
		in       string
		wantUser string
		wantURI  string
		wantObj  string
	}{
		{"/caldav/u1/", "u1", "", ""},
		{"/caldav/u1/cal/", "u1", "", ""},
		{"/caldav/u1/cal/abc/", "u1", "abc", ""},
		{"/caldav/u1/cal/abc", "u1", "abc", ""},
		{"/caldav/u1/cal/abc/e1.ics", "u1", "abc", "e1"},
	}
	for _, c := range cases {
		u, uri, obj := parsePath(c.in)
		if u != c.wantUser || uri != c.wantURI || obj != c.wantObj {
			t.Fatalf("parsePath(%q) = (%q,%q,%q), expected (%q,%q,%q)", c.in, u, uri, obj, c.wantUser, c.wantURI, c.wantObj)
		}
	}
}
