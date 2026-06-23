package subsonic

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// XML responses must render the full payload (nested elements + attributes), not
// just the envelope — real clients like Amperfy speak XML.
func TestWriteXMLRendersPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]any{
		"artists": map[string]any{
			"ignoredArticles": "",
			"index": []any{
				map[string]any{
					"name": "U",
					"artist": []any{
						map[string]any{"id": "ar-1", "name": "Unknown Artist", "albumCount": 1},
					},
				},
			},
		},
	}
	writeXML(rec, "ok", payload)
	body := rec.Body.String()
	for _, want := range []string{
		`status="ok"`, `openSubsonic="true"`,
		`<artists`, `<index name="U">`, `<artist `,
		`name="Unknown Artist"`, `albumCount="1"`, `</subsonic-response>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("xml missing %q in:\n%s", want, body)
		}
	}
}

func TestWriteXMLError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFail(rec, "xml", 40, "Wrong username or password")
	body := rec.Body.String()
	for _, want := range []string{`status="failed"`, `<error `, `code="40"`, `message="Wrong username or password"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("error xml missing %q in:\n%s", want, body)
		}
	}
}

// Scalars with XML-special characters must be escaped in attributes.
func TestWriteXMLEscaping(t *testing.T) {
	rec := httptest.NewRecorder()
	writeXML(rec, "ok", map[string]any{
		"song": map[string]any{"title": `A & B <C> "D"`},
	})
	body := rec.Body.String()
	if !strings.Contains(body, `title="A &amp; B &lt;C&gt; &quot;D&quot;"`) {
		t.Fatalf("attribute not escaped:\n%s", body)
	}
}
