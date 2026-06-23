package carddav

import "testing"

func TestParsePath(t *testing.T) {
	cases := []struct{ in, wantUser, wantURI, wantObj string }{
		{"/carddav/u1/", "u1", "", ""},
		{"/carddav/u1/card/", "u1", "", ""},
		{"/carddav/u1/card/abc/", "u1", "abc", ""},
		{"/carddav/u1/card/abc/c1.vcf", "u1", "abc", "c1"},
	}
	for _, c := range cases {
		u, uri, obj := parsePath(c.in)
		if u != c.wantUser || uri != c.wantURI || obj != c.wantObj {
			t.Fatalf("parsePath(%q) = (%q,%q,%q), expected (%q,%q,%q)", c.in, u, uri, obj, c.wantUser, c.wantURI, c.wantObj)
		}
	}
}
