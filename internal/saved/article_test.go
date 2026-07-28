package saved

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const articleHTML = `<html><head><title>Как я строил NAS</title></head><body>
<article>
<h1>Как я строил NAS</h1>
<p>Длинный осмысленный текст про сборку домашнего сервера. Диски, корпуса, тишина и
прочие радости самостоятельного хостинга. Этот абзац существует, чтобы readability
посчитал страницу настоящей статьёй, а не пустышкой.</p>
<p>Второй абзац с <a href="https://example.com/link">ссылкой</a> и картинкой:
<img src="https://example.com/pic.jpg" alt="фото сервера">.</p>
<p>Третий абзац для веса. Ещё немного слов, чтобы порог длины был пройден уверенно,
и парсер не отбросил контент как слишком короткий.</p>
</article></body></html>`

func TestArticleToMarkdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(articleHTML))
	}))
	defer srv.Close()

	svc, _, q, uid, root := bootstrap(t, 0)
	item, err := svc.Create(context.Background(), uid, srv.URL+"/post", KindArticle, "", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	done := waitStatus(t, q, item.ID, uid, StatusDone)

	if done.Title != "Как я строил NAS" {
		t.Fatalf("title = %q", done.Title)
	}
	year := time.Now().UTC().Format("2006")
	wantPrefix := done.ContentPath.String
	if !strings.HasPrefix(wantPrefix, "") || !strings.Contains(wantPrefix, "/Downloads/articles/"+year+"/") {
		t.Fatalf("content_path = %q, want under Downloads/articles/%s/", wantPrefix, year)
	}
	if filepath.Ext(wantPrefix) != ".md" {
		t.Fatalf("content_path = %q, want .md", wantPrefix)
	}
	b, err := os.ReadFile(filepath.Join(root, done.ContentPath.String))
	if err != nil {
		t.Fatalf("read article: %v", err)
	}
	doc := string(b)
	if !strings.HasPrefix(doc, "---\n") || !strings.Contains(doc, "url: "+srv.URL+"/post") ||
		!strings.Contains(doc, `title: "Как я строил NAS"`) || !strings.Contains(doc, "saved: ") {
		t.Fatalf("frontmatter missing or wrong:\n%s", doc[:min(len(doc), 300)])
	}
	if !strings.Contains(doc, "Длинный осмысленный текст") {
		t.Fatal("article body must be present in markdown")
	}
	if !strings.Contains(doc, "https://example.com/pic.jpg") {
		t.Fatal("images must stay as external links")
	}
	if strings.Contains(doc, "<p>") || strings.Contains(doc, "<article>") {
		t.Fatal("markdown must not contain raw block HTML")
	}

	// Second article with the same title → slug collision → -2 suffix.
	item2, _ := svc.Create(context.Background(), uid, srv.URL+"/post-two", KindArticle, "", "", "")
	done2 := waitStatus(t, q, item2.ID, uid, StatusDone)
	if want := "/Downloads/articles/" + year + "/как-я-строил-nas-2.md"; !strings.HasSuffix(done2.ContentPath.String, want) {
		t.Fatalf("second content_path = %q, want suffix %q", done2.ContentPath.String, want)
	}
}

// failingTransport fails every request, proving that client-supplied
// content_html is processed without a single server-side fetch.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.DeadlineExceeded
}

func TestArticleFromClientHTML(t *testing.T) {
	svc, pool, q, uid, root := bootstrap(t, 0)
	svc.Client = &http.Client{Transport: failingTransport{}}

	clientHTML := `<div><h2>Раздел</h2><p>Текст, извлечённый Readability.js из живого DOM
за пейволом. Сервер эту страницу сам никогда бы не получил.</p></div>`
	item, err := svc.Create(context.Background(), uid, "https://paywalled.example/post", KindArticle, "Статья за пейволом", clientHTML, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	done := waitStatus(t, q, item.ID, uid, StatusDone)

	b, err := os.ReadFile(filepath.Join(root, done.ContentPath.String))
	if err != nil {
		t.Fatalf("read article: %v", err)
	}
	doc := string(b)
	if !strings.Contains(doc, "извлечённый Readability.js") || !strings.Contains(doc, "## Раздел") {
		t.Fatalf("markdown must come from the client HTML:\n%.300s", doc)
	}
	if done.Title != "Статья за пейволом" {
		t.Fatalf("title = %q", done.Title)
	}
	// content_html is transport, not storage: the column is cleared once done.
	var stored *string
	if err := pool.QueryRow(context.Background(), "SELECT content_html FROM saved_items WHERE id=$1", item.ID).Scan(&stored); err != nil {
		t.Fatalf("select content_html: %v", err)
	}
	if stored != nil {
		t.Fatalf("content_html must be cleared after processing, got %q", *stored)
	}
}

func TestArticleUnreadablePageErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>SPA</title></head><body><div id="root"></div></body></html>`))
	}))
	defer srv.Close()

	svc, _, q, uid, _ := bootstrap(t, 0)
	item, _ := svc.Create(context.Background(), uid, srv.URL+"/app", KindArticle, "", "", "")
	errored := waitStatus(t, q, item.ID, uid, StatusError)
	if errored.ErrorMsg == "" {
		t.Fatal("error_msg must explain the failure")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Как я строил NAS":     "как-я-строил-nas",
		"Hello, World! (2026)": "hello-world-2026",
		"  ---  ":              "",
		"C++ и Go: сравнение скорости": "c-и-go-сравнение-скорости",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("слово ", 40)
	if got := Slugify(long); len([]rune(got)) > maxSlugRunes {
		t.Errorf("Slugify(long) = %d runes, want <= %d", len([]rune(got)), maxSlugRunes)
	}
}
