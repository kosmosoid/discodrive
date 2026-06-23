package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/auth"
	"discodrive/internal/db"
	"discodrive/internal/secret"
)

// doPut encodes body as JSON and issues a PUT through the handler with an optional Bearer token.
func doPut(handler http.Handler, path, bearer string, body any) (*httptest.ResponseRecorder, map[string]any) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

// doDelete issues a DELETE through the handler with an optional Bearer token.
func doDelete(handler http.Handler, path, bearer string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

// testCipher builds a real Cipher with a fixed 32-byte key for tests.
func testCipher(t *testing.T) *secret.Cipher {
	t.Helper()
	c, err := secret.New("test-music-key-exactly-32bytes!!")
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	return c
}

// createDirNode inserts a directory node for the given user directly via the db.
// modified_by is left null (it references devices, not users).
func createDirNode(t *testing.T, ctx context.Context, q *db.Queries, uid pgtype.UUID, name string) db.Node {
	t.Helper()
	node, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID: uid,
		Name:   name,
		IsDir:  true,
	})
	if err != nil {
		t.Fatalf("createDirNode: %v", err)
	}
	return node
}

// createFileNode inserts a regular file node for the given user.
// modified_by is left null (it references devices, not users).
func createFileNode(t *testing.T, ctx context.Context, q *db.Queries, uid pgtype.UUID, name string) db.Node {
	t.Helper()
	node, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID: uid,
		Name:   name,
		IsDir:  false,
	})
	if err != nil {
		t.Fatalf("createFileNode: %v", err)
	}
	return node
}

// buildMusicServer constructs a minimal *Server with auth and cipher for music tests.
func buildMusicServer(t *testing.T) (q *db.Queries, svc *auth.Service, s *Server) {
	t.Helper()
	_, q, svc = bootstrapPairingDB(t)
	s = &Server{auth: svc, q: q, cipher: testCipher(t)}
	return
}

// TestMusicSettingsRoundTrip: PUT enabled+folder → GET returns enabled:true, folder populated, hasPassword:false.
func TestMusicSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	q, svc, s := buildMusicServer(t)

	userTok, user, err := svc.Register(ctx, "music1@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	dirNode := createDirNode(t, ctx, q, user.ID, "Music")

	getH := svc.Middleware(http.HandlerFunc(s.handleGetMusicSettings))
	putH := svc.Middleware(http.HandlerFunc(s.handlePutMusicSettings))

	// Initial GET returns zero-value.
	rec, m := doGet(getH, "/me/music", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("initial GET: code=%d body=%v", rec.Code, m)
	}
	if m["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", m["enabled"])
	}
	if m["folder"] != nil {
		t.Fatalf("expected folder=null, got %v", m["folder"])
	}

	// PUT with enabled=true and the dir node.
	folderID := db.UUIDString(dirNode.ID)
	rec, m = doPut(putH, "/me/music", userTok, map[string]any{
		"enabled":      true,
		"folderNodeId": folderID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: code=%d body=%v", rec.Code, m)
	}
	if m["enabled"] != true {
		t.Fatalf("PUT response: expected enabled=true, got %v", m["enabled"])
	}
	if m["hasPassword"] != false {
		t.Fatalf("PUT response: expected hasPassword=false, got %v", m["hasPassword"])
	}
	folder, ok := m["folder"].(map[string]any)
	if !ok || folder == nil {
		t.Fatalf("PUT response: expected folder object, got %v", m["folder"])
	}
	if folder["id"] != folderID {
		t.Fatalf("PUT response: folder.id=%v want %v", folder["id"], folderID)
	}
	if folder["name"] != "Music" {
		t.Fatalf("PUT response: folder.name=%v want Music", folder["name"])
	}

	// GET after PUT returns same shape.
	rec, m = doGet(getH, "/me/music", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after PUT: code=%d body=%v", rec.Code, m)
	}
	if m["enabled"] != true {
		t.Fatalf("GET after PUT: enabled=%v", m["enabled"])
	}
	if m["hasPassword"] != false {
		t.Fatalf("GET after PUT: hasPassword=%v", m["hasPassword"])
	}
	folder2, ok := m["folder"].(map[string]any)
	if !ok || folder2["id"] != folderID {
		t.Fatalf("GET after PUT: folder mismatch: %v", m["folder"])
	}
}

// TestMusicSettingsRejectsForeignFolder: folder owned by another user → PUT returns 404.
func TestMusicSettingsRejectsForeignFolder(t *testing.T) {
	ctx := context.Background()
	q, svc, s := buildMusicServer(t)

	userTok, _, err := svc.Register(ctx, "music2a@x.test", "password12")
	if err != nil {
		t.Fatalf("register user A: %v", err)
	}
	_, otherUser, err := svc.Register(ctx, "music2b@x.test", "password12")
	if err != nil {
		t.Fatalf("register user B: %v", err)
	}

	// Create a directory owned by "other" user.
	otherDir := createDirNode(t, ctx, q, otherUser.ID, "OtherMusic")

	putH := svc.Middleware(http.HandlerFunc(s.handlePutMusicSettings))
	rec, m := doPut(putH, "/me/music", userTok, map[string]any{
		"enabled":      true,
		"folderNodeId": db.UUIDString(otherDir.ID),
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign folder, got %d body=%v", rec.Code, m)
	}
}

// TestMusicSettingsRejectsFile: passing a non-dir node → 400.
func TestMusicSettingsRejectsFile(t *testing.T) {
	ctx := context.Background()
	q, svc, s := buildMusicServer(t)

	userTok, user, err := svc.Register(ctx, "music3@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	fileNode := createFileNode(t, ctx, q, user.ID, "song.mp3")

	putH := svc.Middleware(http.HandlerFunc(s.handlePutMusicSettings))
	rec, m := doPut(putH, "/me/music", userTok, map[string]any{
		"enabled":      true,
		"folderNodeId": db.UUIDString(fileNode.ID),
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for file node, got %d body=%v", rec.Code, m)
	}
}

// TestMusicSettingsTagEditVersioning: PUT tagEditVersioning=false round-trips; omitting defaults to true.
func TestMusicSettingsTagEditVersioning(t *testing.T) {
	ctx := context.Background()
	_, svc, s := buildMusicServer(t)

	userTok, _, err := svc.Register(ctx, "music5@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	getH := svc.Middleware(http.HandlerFunc(s.handleGetMusicSettings))
	putH := svc.Middleware(http.HandlerFunc(s.handlePutMusicSettings))

	// PUT with tagEditVersioning=false.
	rec, m := doPut(putH, "/me/music", userTok, map[string]any{
		"enabled":           true,
		"tagEditVersioning": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT tagEditVersioning=false: code=%d body=%v", rec.Code, m)
	}
	if m["tagEditVersioning"] != false {
		t.Errorf("PUT response: tagEditVersioning = %v, want false", m["tagEditVersioning"])
	}

	// GET returns tagEditVersioning=false.
	rec, m = doGet(getH, "/me/music", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after PUT: code=%d body=%v", rec.Code, m)
	}
	if m["tagEditVersioning"] != false {
		t.Errorf("GET after PUT: tagEditVersioning = %v, want false", m["tagEditVersioning"])
	}

	// Register a second user: GET with no row defaults to tagEditVersioning=true.
	userTok2, _, err := svc.Register(ctx, "music5b@x.test", "password12")
	if err != nil {
		t.Fatalf("register user2: %v", err)
	}
	rec, m = doGet(getH, "/me/music", userTok2)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET no-row: code=%d body=%v", rec.Code, m)
	}
	if m["tagEditVersioning"] != true {
		t.Errorf("GET no-row: tagEditVersioning = %v, want true", m["tagEditVersioning"])
	}

	// PUT omitting tagEditVersioning defaults to true.
	rec, m = doPut(putH, "/me/music", userTok2, map[string]any{
		"enabled": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT omit tagEditVersioning: code=%d body=%v", rec.Code, m)
	}
	if m["tagEditVersioning"] != true {
		t.Errorf("PUT omit: tagEditVersioning = %v, want true", m["tagEditVersioning"])
	}
}

// TestMusicPassword: full password lifecycle — set, verify GET, verify decrypt, delete.
func TestMusicPassword(t *testing.T) {
	ctx := context.Background()
	_, svc, s := buildMusicServer(t)

	userTok, user, err := svc.Register(ctx, "music4@x.test", "password12")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := user.ID

	getH := svc.Middleware(http.HandlerFunc(s.handleGetMusicSettings))
	postPwH := svc.Middleware(http.HandlerFunc(s.handlePostMusicPassword))
	delPwH := svc.Middleware(http.HandlerFunc(s.handleDeleteMusicPassword))

	// POST /me/music/password → returns password + apiKey.
	rec, m := doPost(postPwH, "/me/music/password", userTok, nil)
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

	// GET shows hasPassword:true.
	rec, m = doGet(getH, "/me/music", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after POST password: code=%d body=%v", rec.Code, m)
	}
	if m["hasPassword"] != true {
		t.Fatalf("expected hasPassword=true, got %v", m["hasPassword"])
	}

	// Stored password_cipher decrypts back to the returned plaintext.
	ms, err := s.q.GetMusicSettings(ctx, uid)
	if err != nil {
		t.Fatalf("GetMusicSettings: %v", err)
	}
	if !ms.PasswordCipher.Valid {
		t.Fatal("password_cipher should be set in DB")
	}
	decrypted, err := s.cipher.Decrypt(ms.PasswordCipher.String)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != password {
		t.Fatalf("decrypted=%q want %q", decrypted, password)
	}
	if !ms.ApiKey.Valid || ms.ApiKey.String != apiKey {
		t.Fatalf("api_key in DB=%q want %q", ms.ApiKey.String, apiKey)
	}

	// DELETE /me/music/password → hasPassword:false.
	rec, m = doDelete(delPwH, "/me/music/password", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE password: code=%d body=%v", rec.Code, m)
	}
	rec, m = doGet(getH, "/me/music", userTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after DELETE: code=%d body=%v", rec.Code, m)
	}
	if m["hasPassword"] != false {
		t.Fatalf("expected hasPassword=false after DELETE, got %v", m["hasPassword"])
	}
}
