package dav_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/dav"
	"discodrive/internal/db"
)

const ics1 = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//EN\r\nBEGIN:VEVENT\r\nUID:e1\r\nSUMMARY:A\r\nDTSTART:20260612T120000Z\r\nX-APPLE-CUSTOM:keepme\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
const vcf1 = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Тест\r\nEMAIL:t@e.com\r\nUID:c1\r\nEND:VCARD\r\n"

func setup(t *testing.T) (*dav.Service, *db.Queries, string) {
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
	return dav.NewService(pool), q, db.UUIDString(u.ID)
}

func TestCalendarRoundTripAndCtag(t *testing.T) {
	ctx := context.Background()
	svc, q, userID := setup(t)
	cal, err := svc.CreateCalendar(ctx, userID, "Личный", "#abc")
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	if cal.Uri == "" {
		t.Fatal("uri is empty")
	}
	calID := db.UUIDString(cal.ID)
	etag, err := svc.PutCalendarObject(ctx, calID, "e1", ics1)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if etag == "" {
		t.Fatal("etag is empty")
	}
	data, gotEtag, err := svc.GetCalendarObject(ctx, calID, "e1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if data != ics1 {
		t.Fatalf("round-trip broken:\n%q\n!=\n%q", data, ics1)
	}
	if gotEtag != etag {
		t.Fatalf("etag differs: %q != %q", gotEtag, etag)
	}
	c1, _ := q.GetCalendar(ctx, cal.ID)
	if c1.Ctag == 0 {
		t.Fatalf("ctag did not grow after Put: %d", c1.Ctag)
	}
	objs, _ := svc.ListCalendarObjects(ctx, calID)
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	if err := svc.DeleteCalendarObject(ctx, calID, "e1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	c2, _ := q.GetCalendar(ctx, cal.ID)
	if c2.Ctag <= c1.Ctag {
		t.Fatalf("ctag did not grow after Delete: %d <= %d", c2.Ctag, c1.Ctag)
	}
	if _, _, err := svc.GetCalendarObject(ctx, calID, "e1"); err != dav.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestEnsureDefaultCalendarIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _, userID := setup(t)
	a, err := svc.EnsureDefaultCalendar(ctx, userID)
	if err != nil {
		t.Fatalf("Ensure1: %v", err)
	}
	b, err := svc.EnsureDefaultCalendar(ctx, userID)
	if err != nil {
		t.Fatalf("Ensure2: %v", err)
	}
	if db.UUIDString(a.ID) != db.UUIDString(b.ID) {
		t.Fatal("EnsureDefault created a second calendar")
	}
}

func TestEnsureDefaultTaskList(t *testing.T) {
	ctx := context.Background()
	svc, _, userID := setup(t)

	// no VTODO collection yet → a new one is created with Components containing VTODO
	a, err := svc.EnsureDefaultTaskList(ctx, userID)
	if err != nil {
		t.Fatalf("Ensure1: %v", err)
	}
	if !strings.Contains(a.Components, "VTODO") {
		t.Fatalf("expected Components with VTODO, got %q", a.Components)
	}

	// idempotent: second call returns the same collection
	b, err := svc.EnsureDefaultTaskList(ctx, userID)
	if err != nil {
		t.Fatalf("Ensure2: %v", err)
	}
	if db.UUIDString(a.ID) != db.UUIDString(b.ID) {
		t.Fatal("EnsureDefaultTaskList created a second list")
	}

	// an existing VTODO collection (e.g. from a device) is returned as-is
	existing, err := svc.CreateCalendarWithURI(ctx, userID, "device-vtodo", "Reminders", "VTODO")
	if err != nil {
		t.Fatalf("CreateCalendarWithURI: %v", err)
	}
	_ = existing
	got, err := svc.EnsureDefaultTaskList(ctx, userID)
	if err != nil {
		t.Fatalf("Ensure3: %v", err)
	}
	// must return the FIRST VTODO by created_at (created in Ensure1), not the second
	if db.UUIDString(got.ID) != db.UUIDString(a.ID) {
		t.Fatalf("expected the first VTODO collection %s, got %s", db.UUIDString(a.ID), db.UUIDString(got.ID))
	}
}

func TestSetCalendarColor(t *testing.T) {
	ctx := context.Background()
	svc, _, userID := setup(t)
	c, err := svc.CreateCalendar(ctx, userID, "Работа", "")
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	if err := svc.SetCalendarColor(ctx, userID, db.UUIDString(c.ID), "#22d3ee"); err != nil {
		t.Fatalf("SetCalendarColor: %v", err)
	}
	got, err := svc.GetCalendar(ctx, db.UUIDString(c.ID))
	if err != nil {
		t.Fatalf("GetCalendar: %v", err)
	}
	if got.Color != "#22d3ee" {
		t.Fatalf("color was not updated: %q", got.Color)
	}
}

func TestEnsureDefaultCalendarPrefersVEvent(t *testing.T) {
	ctx := context.Background()
	svc, _, userID := setup(t)
	// create a VTODO list first, then a VEVENT calendar — the default should be VEVENT
	if _, err := svc.CreateCalendarWithURI(ctx, userID, "tasks-x", "Напоминания", "VTODO"); err != nil {
		t.Fatalf("create VTODO: %v", err)
	}
	vevent, err := svc.CreateCalendarWithURI(ctx, userID, "cal-x", "Календарь", "VEVENT")
	if err != nil {
		t.Fatalf("create VEVENT: %v", err)
	}
	def, err := svc.EnsureDefaultCalendar(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureDefaultCalendar: %v", err)
	}
	if db.UUIDString(def.ID) != db.UUIDString(vevent.ID) {
		t.Fatalf("default must be VEVENT %s, got %s", db.UUIDString(vevent.ID), db.UUIDString(def.ID))
	}
}

func TestAddressbookRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, q, userID := setup(t)
	ab, err := svc.CreateAddressbook(ctx, userID, "Друзья")
	if err != nil {
		t.Fatalf("CreateAddressbook: %v", err)
	}
	abID := db.UUIDString(ab.ID)
	if _, err := svc.PutAddressbookObject(ctx, abID, "c1", vcf1); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, _, err := svc.GetAddressbookObject(ctx, abID, "c1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if data != vcf1 {
		t.Fatalf("round-trip vCard broken")
	}
	a1, _ := q.GetAddressbook(ctx, ab.ID)
	if a1.Ctag == 0 {
		t.Fatal("addressbook ctag did not grow")
	}
	if err := svc.DeleteAddressbookObject(ctx, abID, "c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := svc.GetAddressbookObject(ctx, abID, "c1"); err != dav.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
