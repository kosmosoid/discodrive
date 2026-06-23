package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
	"discodrive/internal/secret"
)

const testKey = "test-music-key-exactly-32bytes!!"
const testPassword = "testpassabc"
const testAPIKey = "apikey123"
const testEmail = "me@x.test"

// setupSubsonic spins up a Postgres test container, runs migrations, creates a user
// with music_settings enabled, and returns a ready Handler plus the test context.
func setupSubsonic(t *testing.T) (*Handler, context.Context) {
	t.Helper()
	ctx := context.Background()

	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("Docker required: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	dsn, _ := pgC.ConnectionString(ctx, "sslmode=disable")
	if err := db.MigrateUp(dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	q := db.New(pool)

	// Create tenant + user.
	tenant, err := q.CreateTenant(ctx, "test")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        testEmail,
		PasswordHash: "irrelevant",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Build cipher and encrypt the test password.
	cipher, err := secret.New(testKey)
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	ct, err := cipher.Encrypt(testPassword)
	if err != nil {
		t.Fatalf("cipher.Encrypt: %v", err)
	}

	// Enable music settings with a known folder_node_id (null is fine for these tests).
	_, err = q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  user.ID,
		Enabled: true,
		// FolderNodeID left zero (null) — not needed for auth tests.
	})
	if err != nil {
		t.Fatalf("UpsertMusicSettings: %v", err)
	}

	// Set credentials: encrypted password + plaintext api key.
	err = q.SetMusicCredentials(ctx, db.SetMusicCredentialsParams{
		UserID:         user.ID,
		PasswordCipher: pgtype.Text{String: ct, Valid: true},
		ApiKey:         pgtype.Text{String: testAPIKey, Valid: true},
	})
	if err != nil {
		t.Fatalf("SetMusicCredentials: %v", err)
	}

	h := New(q, cipher, nil, "", false)
	return h, ctx
}

// subsonicGet sends a GET to the handler and returns the response and parsed JSON map.
func subsonicGet(h *Handler, target string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

// subsonicResponse extracts the inner subsonic-response map from the top-level envelope.
func subsonicResponse(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	inner, _ := m["subsonic-response"].(map[string]any)
	return inner
}

// md5hex computes md5(s) and returns it as a lowercase hex string.
func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestPingJSON(t *testing.T) {
	h, _ := setupSubsonic(t)

	salt := "abc"
	token := md5hex(testPassword + salt)
	url := fmt.Sprintf("/rest/ping.view?u=%s&t=%s&s=%s&c=test&v=1.16.1&f=json", testEmail, token, salt)

	rec, m := subsonicGet(h, url)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	resp := subsonicResponse(m)
	if resp == nil {
		t.Fatalf("no subsonic-response in body: %s", rec.Body.String())
	}
	if resp["status"] != "ok" {
		t.Errorf("status=%v, want ok", resp["status"])
	}
	if resp["openSubsonic"] != true {
		t.Errorf("openSubsonic=%v, want true", resp["openSubsonic"])
	}
}

func TestPingApiKey(t *testing.T) {
	h, _ := setupSubsonic(t)

	url := fmt.Sprintf("/rest/ping?apiKey=%s&f=json&c=test&v=1.16.1", testAPIKey)
	rec, m := subsonicGet(h, url)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	resp := subsonicResponse(m)
	if resp == nil {
		t.Fatalf("no subsonic-response in body: %s", rec.Body.String())
	}
	if resp["status"] != "ok" {
		t.Errorf("status=%v, want ok", resp["status"])
	}
}

func TestPingBadToken(t *testing.T) {
	h, _ := setupSubsonic(t)

	url := fmt.Sprintf("/rest/ping.view?u=%s&t=badtoken&s=abc&c=test&v=1.16.1&f=json", testEmail)
	rec, m := subsonicGet(h, url)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	resp := subsonicResponse(m)
	if resp == nil {
		t.Fatalf("no subsonic-response in body: %s", rec.Body.String())
	}
	if resp["status"] != "failed" {
		t.Errorf("status=%v, want failed", resp["status"])
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("no error object in response: %v", resp)
	}
	// JSON numbers decode as float64.
	if code, _ := errObj["code"].(float64); int(code) != ErrWrongAuth {
		t.Errorf("error.code=%v, want %d", errObj["code"], ErrWrongAuth)
	}
}

func TestPingDisabled(t *testing.T) {
	h, ctx := setupSubsonic(t)

	// Disable music for the test user.
	_, err := h.q.UpsertMusicSettings(ctx, db.UpsertMusicSettingsParams{
		UserID:  mustUserID(t, ctx, h.q, testEmail),
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("UpsertMusicSettings(disabled): %v", err)
	}

	salt := "abc"
	token := md5hex(testPassword + salt)
	url := fmt.Sprintf("/rest/ping.view?u=%s&t=%s&s=%s&c=test&v=1.16.1&f=json", testEmail, token, salt)
	_, m := subsonicGet(h, url)

	resp := subsonicResponse(m)
	if resp == nil {
		t.Fatalf("no subsonic-response")
	}
	if resp["status"] != "failed" {
		t.Errorf("status=%v, want failed", resp["status"])
	}
	errObj, _ := resp["error"].(map[string]any)
	if code, _ := errObj["code"].(float64); int(code) != ErrWrongAuth {
		t.Errorf("error.code=%v, want %d", errObj["code"], ErrWrongAuth)
	}
}

func TestExtensions(t *testing.T) {
	h, _ := setupSubsonic(t)

	url := fmt.Sprintf("/rest/getOpenSubsonicExtensions?apiKey=%s&f=json&c=test&v=1.16.1", testAPIKey)
	rec, m := subsonicGet(h, url)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	resp := subsonicResponse(m)
	if resp == nil {
		t.Fatalf("no subsonic-response")
	}
	if resp["status"] != "ok" {
		t.Errorf("status=%v, want ok", resp["status"])
	}

	exts, _ := resp["openSubsonicExtensions"].([]any)
	if len(exts) == 0 {
		t.Fatalf("openSubsonicExtensions is empty")
	}

	names := map[string]bool{}
	for _, e := range exts {
		if em, ok := e.(map[string]any); ok {
			if n, ok := em["name"].(string); ok {
				names[n] = true
			}
		}
	}
	for _, want := range []string{"formPost", "apiKeyAuth", "songLyrics"} {
		if !names[want] {
			t.Errorf("missing extension %q in %v", want, names)
		}
	}
}

// mustUserID looks up the pgtype.UUID for a user by email; fatal on error.
func mustUserID(t *testing.T, ctx context.Context, q *db.Queries, email string) pgtype.UUID {
	t.Helper()
	u, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetUserByEmail(%q): %v", email, err)
	}
	return u.ID
}
