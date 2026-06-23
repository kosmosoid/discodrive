package carddav_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	godavcarddav "github.com/emersion/go-webdav/carddav"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/carddav"
	"discodrive/internal/dav"
	"discodrive/internal/db"
)

const card = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Иван Петров\r\nEMAIL:ivan@example.com\r\nX-CUSTOM-FIELD:keepme\r\nUID:c1\r\nEND:VCARD\r\n"

func setup(t *testing.T) (http.Handler, string, string) {
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
	pool, _ := pgxpool.New(ctx, dsn)
	t.Cleanup(pool.Close)
	q := db.New(pool)
	tenant, _ := q.CreateTenant(ctx, "t")
	u, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "u@x", PasswordHash: "x", Role: "user"})
	userID := db.UUIDString(u.ID)
	svc := dav.NewService(pool)
	ab, _ := svc.CreateAddressbook(ctx, userID, "Контакты")

	h := &godavcarddav.Handler{Backend: carddav.New(svc), Prefix: "/carddav"}
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := carddav.WithUserID(r.Context(), userID)
		if r.Method == http.MethodPut {
			raw, _ := io.ReadAll(r.Body)
			c = carddav.WithRawBody(c, raw)
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		h.ServeHTTP(w, r.WithContext(c))
	})
	return wrapped, userID, ab.Uri
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "text/vcard; charset=utf-8")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPutGetRoundTrip(t *testing.T) {
	h, userID, uri := setup(t)
	objPath := "/carddav/" + userID + "/card/" + uri + "/c1.vcf"

	put := do(t, h, http.MethodPut, objPath, card)
	if put.Code != http.StatusCreated && put.Code != http.StatusNoContent && put.Code != http.StatusOK {
		t.Fatalf("PUT code=%d body=%s", put.Code, put.Body.String())
	}
	if put.Header().Get("ETag") == "" {
		t.Fatalf("PUT did not return an ETag")
	}

	get := do(t, h, http.MethodGet, objPath, "")
	if get.Code != http.StatusOK {
		t.Fatalf("GET code=%d body=%s", get.Code, get.Body.String())
	}
	for _, want := range []string{"FN:Иван Петров", "EMAIL:ivan@example.com", "X-CUSTOM-FIELD:keepme"} {
		if !strings.Contains(get.Body.String(), want) {
			t.Fatalf("GET output is missing %q:\n%s", want, get.Body.String())
		}
	}
}

func TestDeleteThen404(t *testing.T) {
	h, userID, uri := setup(t)
	objPath := "/carddav/" + userID + "/card/" + uri + "/c1.vcf"
	do(t, h, http.MethodPut, objPath, card)
	del := do(t, h, http.MethodDelete, objPath, "")
	if del.Code != http.StatusNoContent && del.Code != http.StatusOK {
		t.Fatalf("DELETE code=%d", del.Code)
	}
	get := do(t, h, http.MethodGet, objPath, "")
	if get.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", get.Code)
	}
}

// setupTwo spins up a test CardDAV server with two users (owner and grantee).
// Returns a handler with per-request auth (X-User-ID header), dav.Service, ownerID, granteeID.
func setupTwo(t *testing.T) (http.Handler, *dav.Service, string, string) {
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
	pool, _ := pgxpool.New(ctx, dsn)
	t.Cleanup(pool.Close)
	q := db.New(pool)
	tenant, _ := q.CreateTenant(ctx, "t")
	owner, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "owner@x", PasswordHash: "x", Role: "user"})
	grantee, _ := q.CreateUser(ctx, db.CreateUserParams{TenantID: tenant.ID, Email: "grantee@x", PasswordHash: "x", Role: "user"})
	ownerID := db.UUIDString(owner.ID)
	granteeID := db.UUIDString(grantee.ID)
	svc := dav.NewService(pool)

	h := &godavcarddav.Handler{Backend: carddav.New(svc), Prefix: "/carddav"}
	// handler reads X-User-ID from the request for test auth
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := r.Header.Get("X-User-ID")
		c := carddav.WithUserID(r.Context(), uid)
		if r.Method == http.MethodPut {
			raw, _ := io.ReadAll(r.Body)
			c = carddav.WithRawBody(c, raw)
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		h.ServeHTTP(w, r.WithContext(c))
	})
	return wrapped, svc, ownerID, granteeID
}

// doAs sends a request on behalf of the specified user.
func doAs(t *testing.T, h http.Handler, uid, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-User-ID", uid)
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "text/vcard; charset=utf-8")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSharedAddressbookVisibleAndWritable(t *testing.T) {
	ctx := context.Background()
	h, svc, ownerID, granteeID := setupTwo(t)

	// owner: create address book and add a vCard
	ab, err := svc.CreateAddressbook(ctx, ownerID, "Семейная книга")
	if err != nil {
		t.Fatalf("CreateAddressbook: %v", err)
	}
	abURI := ab.Uri
	abID := db.UUIDString(ab.ID)

	ownerObjPath := "/carddav/" + ownerID + "/card/" + abURI + "/c1.vcf"
	put := doAs(t, h, ownerID, http.MethodPut, ownerObjPath, card)
	if put.Code != http.StatusCreated && put.Code != http.StatusNoContent && put.Code != http.StatusOK {
		t.Fatalf("owner PUT code=%d body=%s", put.Code, put.Body.String())
	}

	// share: owner shares with grantee
	if _, err := svc.ShareAddressbook(ctx, ownerID, abID, "grantee@x"); err != nil {
		t.Fatalf("ShareAddressbook: %v", err)
	}

	// PROPFIND grantee's home-set at Depth:1 — must include the owner's URI
	propfindBody := `<?xml version="1.0" encoding="UTF-8"?><propfind xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav"><prop><C:addressbook-description/><displayname/><resourcetype/></prop></propfind>`
	homeSetReq := httptest.NewRequest("PROPFIND", "/carddav/"+granteeID+"/card/", strings.NewReader(propfindBody))
	homeSetReq.Header.Set("X-User-ID", granteeID)
	homeSetReq.Header.Set("Depth", "1")
	homeSetReq.Header.Set("Content-Type", "application/xml")
	homeSetRec := httptest.NewRecorder()
	h.ServeHTTP(homeSetRec, homeSetReq)
	if homeSetRec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND home-set code=%d body=%s", homeSetRec.Code, homeSetRec.Body.String())
	}
	if !strings.Contains(homeSetRec.Body.String(), abURI) {
		t.Fatalf("PROPFIND does not contain owner uri %q:\n%s", abURI, homeSetRec.Body.String())
	}

	// GET the object as grantee — path is under the owner's namespace
	// (toDAVAddressbook builds the path with uid=ownerID, so grantee accesses /carddav/{ownerID}/card/{uri}/{obj})
	getObjPath := "/carddav/" + ownerID + "/card/" + abURI + "/c1.vcf"
	getRec := doAs(t, h, granteeID, http.MethodGet, getObjPath, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("grantee GET code=%d body=%s", getRec.Code, getRec.Body.String())
	}

	// PUT a new object as grantee → 201/204/200
	newCard := strings.ReplaceAll(card, "UID:c1", "UID:c2")
	newObjPath := "/carddav/" + ownerID + "/card/" + abURI + "/c2.vcf"
	putRec := doAs(t, h, granteeID, http.MethodPut, newObjPath, newCard)
	if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent && putRec.Code != http.StatusOK {
		t.Fatalf("grantee PUT code=%d body=%s", putRec.Code, putRec.Body.String())
	}
}
