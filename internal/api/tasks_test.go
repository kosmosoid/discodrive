package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/dav"
	"discodrive/internal/db"
)

func setupTasks(t *testing.T) (*dav.Service, string, context.Context) {
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
	cal, err := svc.EnsureDefaultTaskList(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureDefaultTaskList: %v", err)
	}
	return svc, db.UUIDString(cal.ID), ctx
}

// buildTaskCal builds a VCALENDAR+VTODO from a form (mirroring handleCreateTask) and serializes it.
func buildTaskCal(uid string, f taskForm) string {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//t//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")
	todo := ical.NewComponent(ical.CompToDo)
	todo.Props.SetText(ical.PropUID, uid)
	todo.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	applyTaskForm(todo, f)
	cal.Children = append(cal.Children, todo)
	var b strings.Builder
	_ = ical.NewEncoder(&b).Encode(cal)
	return b.String()
}

func TestTaskBuildAndForm(t *testing.T) {
	svc, calID, ctx := setupTasks(t)
	raw := buildTaskCal("t1", taskForm{
		Summary:  "Купить молоко",
		Notes:    "2 литра",
		Due:      "2026-06-20T18:00:00Z",
		Priority: 5,
	})
	if _, err := svc.PutCalendarObject(ctx, calID, "t1", raw); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, _, err := svc.GetCalendarObject(ctx, calID, "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	f := taskToForm("t1", cal)
	if f.Summary != "Купить молоко" || f.Notes != "2 литра" || f.Priority != 5 {
		t.Fatalf("form mismatch: %+v", f)
	}
	if f.Due == "" {
		t.Fatalf("due date lost: %+v", f)
	}
	if f.Completed {
		t.Fatalf("newly created task should not be completed")
	}
}

func TestTaskPriorityNoValueText(t *testing.T) {
	raw := buildTaskCal("t2", taskForm{Summary: "X", Priority: 5})
	if !strings.Contains(raw, "PRIORITY:5") {
		t.Fatalf("expected PRIORITY:5, got:\n%s", raw)
	}
	if strings.Contains(raw, "VALUE=TEXT") {
		t.Fatalf("PRIORITY serialized as TEXT:\n%s", raw)
	}
}

func TestSetTaskCompleted(t *testing.T) {
	raw := buildTaskCal("t3", taskForm{Summary: "X"})
	cal, _ := ical.NewDecoder(strings.NewReader(raw)).Decode()
	for _, c := range cal.Children {
		if c.Name == ical.CompToDo {
			setTaskCompleted(c, true)
		}
	}
	var b strings.Builder
	_ = ical.NewEncoder(&b).Encode(cal)
	out := b.String()
	if !strings.Contains(out, "STATUS:COMPLETED") {
		t.Fatalf("expected STATUS:COMPLETED:\n%s", out)
	}
	if strings.Contains(out, "VALUE=TEXT") {
		t.Fatalf("status/percent serialized as TEXT:\n%s", out)
	}
	// unset
	for _, c := range cal.Children {
		if c.Name == ical.CompToDo {
			setTaskCompleted(c, false)
		}
	}
	var b2 strings.Builder
	_ = ical.NewEncoder(&b2).Encode(cal)
	out2 := b2.String()
	if !strings.Contains(out2, "STATUS:NEEDS-ACTION") {
		t.Fatalf("expected STATUS:NEEDS-ACTION:\n%s", out2)
	}
	if strings.Contains(out2, "COMPLETED:") {
		t.Fatalf("COMPLETED property should have been removed:\n%s", out2)
	}
}

func TestTaskModifyExistingPreservesUnknown(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//EN\r\nBEGIN:VTODO\r\nUID:t4\r\nDTSTAMP:20260611T000000Z\r\nSUMMARY:Old\r\nX-APPLE-FOO:bar\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	cal, _ := ical.NewDecoder(strings.NewReader(raw)).Decode()
	for _, c := range cal.Children {
		if c.Name == ical.CompToDo {
			applyTaskForm(c, taskForm{Summary: "New"})
		}
	}
	var b strings.Builder
	_ = ical.NewEncoder(&b).Encode(cal)
	out := b.String()
	if !strings.Contains(out, "X-APPLE-FOO:bar") {
		t.Fatalf("X-APPLE-FOO lost:\n%s", out)
	}
	if !strings.Contains(out, "VALARM") {
		t.Fatalf("VALARM lost:\n%s", out)
	}
	if !strings.Contains(out, "SUMMARY:New") {
		t.Fatalf("summary not updated:\n%s", out)
	}
}
