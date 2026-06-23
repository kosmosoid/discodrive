package api

import (
	"context"
	"net/http"
	"testing"

	"discodrive/internal/auth"
	"discodrive/internal/db"
)

// buildEbookServer constructs a minimal *Server with auth and cipher for ebook tests.
func buildEbookServer(t *testing.T) (q *db.Queries, svc *auth.Service, s *Server) {
	t.Helper()
	_, q, svc = bootstrapPairingDB(t)
	s = &Server{auth: svc, q: q, cipher: testCipher(t)}
	return
}

// TestEbookSettingsGetNoRow: GET without any prior upsert → zero-value response.
func TestEbookSettingsGetNoRow(t *testing.T) {
	ctx := context.Background()
	_, svc, s := buildEbookServer(t)

	userTok, _, err := svc.Register(ctx, "ebook1@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	getH := svc.Middleware(http.HandlerFunc(s.handleGetEbookSettings))
	rec, m := doGet(getH, "/me/ebooks", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: code=%d body=%v", rec.Code, m)
	}
	if m["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", m["enabled"])
	}
	if m["folder"] != nil {
		t.Fatalf("expected folder=null, got %v", m["folder"])
	}
	if m["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false, got %v", m["hasPassword"])
	}
	if m["hasApiKey"] != false {
		t.Fatalf("expected hasApiKey=false, got %v", m["hasApiKey"])
	}
}

// TestEbookSettingsRejectsForeignFolder: folder owned by another user → PUT returns 404.
func TestEbookSettingsRejectsForeignFolder(t *testing.T) {
	ctx := context.Background()
	q, svc, s := buildEbookServer(t)

	userTok, _, err := svc.Register(ctx, "ebook2a@x.test", "password12")
	if err != nil {
		t.Fatalf("register user A: %v", err)
	}
	_, otherUser, err := svc.Register(ctx, "ebook2b@x.test", "password12")
	if err != nil {
		t.Fatalf("register user B: %v", err)
	}

	// Create a directory owned by "other" user.
	otherDir := createDirNode(t, ctx, q, otherUser.ID, "OtherBooks")

	putH := svc.Middleware(http.HandlerFunc(s.handlePutEbookSettings))
	rec, m := doPut(putH, "/me/ebooks", userTok, map[string]any{
		"enabled":      true,
		"folderNodeId": db.UUIDString(otherDir.ID),
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign folder, got %d body=%v", rec.Code, m)
	}
}

// TestEbookSettingsPutValidDir: PUT with a valid dir → enabled+folder returned.
func TestEbookSettingsPutValidDir(t *testing.T) {
	ctx := context.Background()
	q, svc, s := buildEbookServer(t)

	userTok, user, err := svc.Register(ctx, "ebook3@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	dirNode := createDirNode(t, ctx, q, user.ID, "Books")

	putH := svc.Middleware(http.HandlerFunc(s.handlePutEbookSettings))
	folderID := db.UUIDString(dirNode.ID)
	rec, m := doPut(putH, "/me/ebooks", userTok, map[string]any{
		"enabled":      true,
		"folderNodeId": folderID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: code=%d body=%v", rec.Code, m)
	}
	if m["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", m["enabled"])
	}
	if m["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false, got %v", m["hasPassword"])
	}
	if m["hasApiKey"] != false {
		t.Fatalf("expected hasApiKey=false, got %v", m["hasApiKey"])
	}
	folder, ok := m["folder"].(map[string]any)
	if !ok || folder == nil {
		t.Fatalf("expected folder object, got %v", m["folder"])
	}
	if folder["id"] != folderID {
		t.Fatalf("folder.id=%v want %v", folder["id"], folderID)
	}
	if folder["name"] != "Books" {
		t.Fatalf("folder.name=%v want Books", folder["name"])
	}
}

// TestEbookPassword: POST generates password+apiKey; subsequent GET shows hasPassword:true.
func TestEbookPassword(t *testing.T) {
	ctx := context.Background()
	_, svc, s := buildEbookServer(t)

	userTok, user, err := svc.Register(ctx, "ebook4@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := user.ID

	getH := svc.Middleware(http.HandlerFunc(s.handleGetEbookSettings))
	postPwH := svc.Middleware(http.HandlerFunc(s.handlePostEbookPassword))
	delPwH := svc.Middleware(http.HandlerFunc(s.handleDeleteEbookPassword))

	// POST /me/ebooks/password → returns password + apiKey.
	rec, m := doPost(postPwH, "/me/ebooks/password", userTok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST password: code=%d body=%v", rec.Code, m)
	}
	password, _ := m["password"].(string)
	apiKey, _ := m["apiKey"].(string)
	if password == "" {
		t.Fatal("POST password: empty password in response")
	}
	if apiKey == "" {
		t.Fatal("POST password: empty apiKey in response")
	}

	// GET shows hasPassword:true and hasApiKey:true.
	rec, m = doGet(getH, "/me/ebooks", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after POST password: code=%d body=%v", rec.Code, m)
	}
	if m["hasPassword"] != true {
		t.Fatalf("expected hasPassword=true, got %v", m["hasPassword"])
	}
	if m["hasApiKey"] != true {
		t.Fatalf("expected hasApiKey=true, got %v", m["hasApiKey"])
	}

	// Stored password_cipher decrypts back to the returned plaintext.
	es, err := s.q.GetEbookSettings(ctx, uid)
	if err != nil {
		t.Fatalf("GetEbookSettings: %v", err)
	}
	if !es.PasswordCipher.Valid {
		t.Fatal("password_cipher should be set in DB")
	}
	decrypted, err := s.cipher.Decrypt(es.PasswordCipher.String)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != password {
		t.Fatalf("decrypted=%q want %q", decrypted, password)
	}
	if !es.ApiKey.Valid || es.ApiKey.String != apiKey {
		t.Fatalf("api_key in DB=%q want %q", es.ApiKey.String, apiKey)
	}

	// DELETE /me/ebooks/password → hasPassword:false, hasApiKey:false.
	rec, m = doDelete(delPwH, "/me/ebooks/password", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE password: code=%d body=%v", rec.Code, m)
	}
	rec, m = doGet(getH, "/me/ebooks", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after DELETE: code=%d body=%v", rec.Code, m)
	}
	if m["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false after DELETE, got %v", m["hasPassword"])
	}
	if m["hasApiKey"] != false {
		t.Fatalf("expected hasApiKey=false after DELETE, got %v", m["hasApiKey"])
	}
}
