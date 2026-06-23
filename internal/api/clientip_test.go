package api

import (
	"net/http"
	"testing"
)

func TestClientIP_XFFTrust(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"no xff uses remote", "203.0.113.5:5000", "", "203.0.113.5"},
		{"public peer ignores spoofed xff", "203.0.113.5:5000", "1.2.3.4", "203.0.113.5"},
		{"trusted proxy uses last xff", "10.0.0.2:5000", "9.9.9.9", "9.9.9.9"},
		{"trusted proxy takes rightmost (real) entry", "10.0.0.2:5000", "1.2.3.4, 9.9.9.9", "9.9.9.9"},
		{"loopback proxy trusted", "127.0.0.1:5000", "9.9.9.9", "9.9.9.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: c.remoteAddr, Header: http.Header{}}
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(r); got != c.want {
				t.Fatalf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}
