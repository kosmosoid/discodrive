package worker

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"discodrive/internal/db"
	"discodrive/internal/notify"
)

// mailbox is an email channel that hands every sent message to the test.
type mailbox struct{ sent chan notify.Message }

func (m *mailbox) Name() string { return "email" }

func (m *mailbox) Send(_ context.Context, msg notify.Message) error {
	m.sent <- msg
	return nil
}

// waitMail returns the next message, or fails if none arrives: Emit delivers in a
// goroutine, so a mail that is on its way must be given a moment.
func (m *mailbox) waitMail(t *testing.T) notify.Message {
	t.Helper()
	select {
	case msg := <-m.sent:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("expected an alert email, got none")
		return notify.Message{}
	}
}

func (m *mailbox) expectSilence(t *testing.T) {
	t.Helper()
	select {
	case msg := <-m.sent:
		t.Fatalf("expected no email, got %q", msg.Subject)
	case <-time.After(300 * time.Millisecond):
	}
}

// The cap filling up must reach the administrators — and only once per level, or a
// server sitting at 95% mails them every 15 minutes until they stop reading.
func TestStorageAlert_MailsAdminsOncePerLevel(t *testing.T) {
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		tcpostgres.BasicWaitStrategies())
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
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	tenant, err := q.CreateTenant(ctx, "t")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if _, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID: tenant.ID, Email: "admin@x", PasswordHash: "x", Role: "admin",
	}); err != nil {
		t.Fatalf("admin: %v", err)
	}
	// A plain user must not be mailed: the alert asks for action only an admin can take.
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID: tenant.ID, Email: "user@x", PasswordHash: "x", Role: "user",
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}

	box := &mailbox{sent: make(chan notify.Message, 8)}
	w := &Worker{
		root: t.TempDir(), q: q, notify: notify.New(q, box),
		cfg: Config{StorageTotal: 1000},
		// Plenty of disk left, so the cap is the tighter of the two limits.
		disk: func(string) (uint64, uint64, error) { return 1_000_000, 900_000, nil },
	}

	// 82% of the cap used — 18% free is under the 20% warning threshold.
	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (user_id, name, is_dir, size) VALUES ($1, 'big.bin', false, 820)`, user.ID); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := w.storageAlert(ctx); err != nil {
		t.Fatalf("storageAlert: %v", err)
	}
	if msg := box.waitMail(t); msg.To != "admin@x" {
		t.Fatalf("alert went to %q, want admin@x", msg.To)
	}
	box.expectSilence(t) // the plain user gets nothing

	// Nothing changed since the last tick: the admins must not hear about it again.
	if err := w.storageAlert(ctx); err != nil {
		t.Fatalf("storageAlert (repeat): %v", err)
	}
	box.expectSilence(t)

	// It got worse — crossing into critical is worth a second mail.
	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (user_id, name, is_dir, size) VALUES ($1, 'bigger.bin', false, 100)`, user.ID); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := w.storageAlert(ctx); err != nil {
		t.Fatalf("storageAlert (critical): %v", err)
	}
	box.waitMail(t)

	// Space was freed: the level is recorded again, but recovery is not mailed about.
	if _, err := pool.Exec(ctx, `DELETE FROM nodes WHERE user_id = $1`, user.ID); err != nil {
		t.Fatalf("delete nodes: %v", err)
	}
	if err := w.storageAlert(ctx); err != nil {
		t.Fatalf("storageAlert (recovered): %v", err)
	}
	box.expectSilence(t)
	if lvl := w.storedAlertLevel(ctx); lvl != 0 {
		t.Fatalf("level after recovery = %d, want 0", lvl)
	}

	// And once it fills up again, the next crossing mails again.
	if _, err := pool.Exec(ctx,
		`INSERT INTO nodes (user_id, name, is_dir, size) VALUES ($1, 'again.bin', false, 950)`, user.ID); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := w.storageAlert(ctx); err != nil {
		t.Fatalf("storageAlert (refilled): %v", err)
	}
	box.waitMail(t)
}

// With no cap configured the job must still watch the physical disk — that is the only
// limit a server without STORAGE_TOTAL_GB has.
func TestStorageAlert_WatchesDiskWithoutCap(t *testing.T) {
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		tcpostgres.BasicWaitStrategies())
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
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	tenant, _ := q.CreateTenant(ctx, "t")
	if _, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID: tenant.ID, Email: "admin@x", PasswordHash: "x", Role: "admin",
	}); err != nil {
		t.Fatalf("admin: %v", err)
	}

	box := &mailbox{sent: make(chan notify.Message, 4)}
	w := &Worker{
		root: t.TempDir(), q: q, notify: notify.New(q, box),
		cfg:  Config{StorageTotal: 0},
		disk: func(string) (uint64, uint64, error) { return 1000, 50, nil }, // 5% free
	}
	if err := w.storageAlert(ctx); err != nil {
		t.Fatalf("storageAlert: %v", err)
	}
	box.waitMail(t)
}
