package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/dav"
	"discodrive/internal/db"
)

func setupContacts(t *testing.T) (*dav.Service, string, context.Context) {
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
	ab, err := svc.CreateAddressbook(ctx, userID, "Контакты")
	if err != nil {
		t.Fatalf("CreateAddressbook: %v", err)
	}
	return svc, db.UUIDString(ab.ID), ctx
}

func TestContactCreateAndForm(t *testing.T) {
	svc, abID, ctx := setupContacts(t)

	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "3.0")
	card.SetValue(vcard.FieldUID, "c1")
	applyForm(card, contactForm{
		FullName: "Иван Петров",
		Family:   "Петров",
		Given:    "Иван",
		Emails:   []typedValue{{Type: "home", Value: "ivan@e.com"}},
		Phones:   []typedValue{{Value: "+79990001122"}},
		Org:      "Acme",
	})
	var b strings.Builder
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := svc.PutAddressbookObject(ctx, abID, "c1", b.String()); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, _, err := svc.GetAddressbookObject(ctx, abID, "c1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := vcard.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	form := cardToForm("c1", got)
	if form.FullName != "Иван Петров" || form.Org != "Acme" || len(form.Emails) != 1 || form.Emails[0].Value != "ivan@e.com" {
		t.Fatalf("form did not match: %+v", form)
	}
}

func TestModifyExistingPreservesUnknown(t *testing.T) {
	svc, abID, ctx := setupContacts(t)

	raw := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Old Name\r\nUID:c2\r\nX-FOO:bar\r\nPHOTO;VALUE=URI:data:image/jpeg\r\nEND:VCARD\r\n"
	if _, err := svc.PutAddressbookObject(ctx, abID, "c2", raw); err != nil {
		t.Fatalf("Put raw: %v", err)
	}
	data, _, err := svc.GetAddressbookObject(ctx, abID, "c2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	card, err := vcard.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	applyForm(card, contactForm{FullName: "New Name", Family: "Name", Given: "New"})
	var b strings.Builder
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := svc.PutAddressbookObject(ctx, abID, "c2", b.String()); err != nil {
		t.Fatalf("Put updated: %v", err)
	}
	out, _, err := svc.GetAddressbookObject(ctx, abID, "c2")
	if err != nil {
		t.Fatalf("Get updated: %v", err)
	}
	if !strings.Contains(out, "X-FOO:bar") {
		t.Fatalf("X-FOO lost during modify-existing:\n%s", out)
	}
	if !strings.Contains(out, "PHOTO") {
		t.Fatalf("PHOTO lost during modify-existing:\n%s", out)
	}
	if !strings.Contains(out, "New Name") {
		t.Fatalf("new name was not written:\n%s", out)
	}
}
