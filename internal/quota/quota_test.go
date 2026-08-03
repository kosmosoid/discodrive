package quota

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// pgUUID is a placeholder id for the nil-checker paths, which never touch the database.
func pgUUID() pgtype.UUID { return pgtype.UUID{} }

func ptr[T any](v T) *T { return &v }

// A source that stops exactly at the allowance is a legal write: the reader must end
// with EOF, not with ErrExceeded. Getting this wrong rejects every upload that fills
// the quota to the last byte.
func TestLimitReader_ExactlyAtAllowance(t *testing.T) {
	r := &limitReader{r: strings.NewReader("0123456789"), left: 10, allowance: 10}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "0123456789" {
		t.Fatalf("got %q", got)
	}
}

// One byte past the allowance must fail, and must fail as ErrExceeded — an io.EOF here
// would publish a truncated file as if the client had sent exactly that much.
func TestLimitReader_OneByteOver(t *testing.T) {
	r := &limitReader{r: strings.NewReader("0123456789X"), left: 10, allowance: 10}
	_, err := io.ReadAll(r)
	if !errors.Is(err, ErrExceeded) {
		t.Fatalf("want ErrExceeded, got %v", err)
	}
}

// A reader handing out one byte at a time must not slip past the limit either.
func TestLimitReader_ByteAtATime(t *testing.T) {
	r := &limitReader{r: &iotest{s: "abcdef"}, left: 3, allowance: 3}
	n, err := io.Copy(io.Discard, r)
	if !errors.Is(err, ErrExceeded) {
		t.Fatalf("want ErrExceeded after %d bytes, got %v", n, err)
	}
	if n > 4 { // 3 allowed + the one that proves the overrun
		t.Fatalf("read %d bytes past a 3-byte allowance", n)
	}
}

// iotest yields one byte per Read.
type iotest struct {
	s string
	i int
}

func (t *iotest) Read(p []byte) (int, error) {
	if t.i >= len(t.s) {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = t.s[t.i]
	t.i++
	return 1, nil
}

// A nil Checker means "no limits configured": every method has to stay usable so the
// storage layer can hold an optional checker without nil guards everywhere.
func TestNilChecker(t *testing.T) {
	var c *Checker
	if err := c.Check(t.Context(), pgUUID(), 1<<40); err != nil {
		t.Fatalf("Check on nil checker: %v", err)
	}
	r, err := c.Reader(t.Context(), pgUUID(), strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Reader on nil checker: %v", err)
	}
	if got, _ := io.ReadAll(r); string(got) != "x" {
		t.Fatalf("nil checker must not wrap the reader, got %q", got)
	}
	if err := c.CheckAssign(t.Context(), pgUUID(), ptr(int64(1<<40))); err != nil {
		t.Fatalf("CheckAssign on nil checker: %v", err)
	}
	if _, capped, err := c.Assignable(t.Context(), pgUUID()); err != nil || capped {
		t.Fatalf("Assignable on nil checker: capped=%v err=%v", capped, err)
	}
	if c.Total() != 0 {
		t.Fatalf("Total on nil checker = %d, want 0", c.Total())
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:            "0 B",
		512:          "512 B",
		1024:         "1.0 KiB",
		1 << 30:      "1.0 GiB",
		3 * 1 << 40:  "3.0 TiB",
		Unlimited:    "unlimited",
		1536:         "1.5 KiB",
		107374182400: "100.0 GiB",
	}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestRemainingFloorsAtZero(t *testing.T) {
	// A quota lowered below what the user already stores must read as "no room left",
	// never as a negative allowance — which would win min() and read as unlimited.
	if got := remaining(100, 250); got != 0 {
		t.Fatalf("remaining(100, 250) = %d, want 0", got)
	}
	if got := remaining(100, 40); got != 60 {
		t.Fatalf("remaining(100, 40) = %d, want 60", got)
	}
}
