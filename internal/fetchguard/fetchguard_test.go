package fetchguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateURLScheme(t *testing.T) {
	if err := ValidateURL("ftp://example.com/x"); err == nil {
		t.Error("ftp should be rejected")
	}
	if err := ValidateURL("file:///etc/passwd"); err == nil {
		t.Error("file scheme should be rejected")
	}
}

func TestValidateURLPrivate(t *testing.T) {
	for _, u := range []string{
		"http://127.0.0.1/feed",
		"http://localhost/feed",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/feed",
		"http://192.168.1.1/feed",
	} {
		if err := ValidateURL(u); err == nil {
			t.Errorf("%s should be rejected as private/loopback/link-local", u)
		}
	}
}

func TestValidateURLPublicLiteral(t *testing.T) {
	// 8.8.8.8 is a public IP literal; net.LookupIP returns it directly without
	// DNS, so the result is deterministic even in offline environments.
	if err := ValidateURL("https://8.8.8.8/feed.xml"); err != nil {
		t.Errorf("public IP literal should be accepted: %v", err)
	}
}

func TestNewClientBlocksLoopbackDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := NewClient(0).Get(srv.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("dial to 127.0.0.1 should be blocked by the dialer, got %v", err)
	}
}
