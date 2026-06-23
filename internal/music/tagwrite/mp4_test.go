package tagwrite

import (
	"errors"
	"testing"
)

func TestM4A_RoundTripOrReadOnly(t *testing.T) {
	path := copyToTemp(t, "sample.m4a")
	w, writable := For("m4a")
	if w == nil {
		t.Fatal("m4a must at least be readable")
	}
	// Read must always work.
	if _, _, err := w.Read(path); err != nil {
		t.Fatalf("Read: %v", err)
	}
	err := w.Apply(path, Tags{Title: strp("X")}, CoverKeep, nil)
	if writable {
		if err != nil {
			t.Fatalf("writable m4a Apply: %v", err)
		}
		got, _, _ := w.Read(path)
		if got.Title == nil || *got.Title != "X" {
			t.Errorf("Title = %v, want X", got.Title)
		}
	} else {
		if !errors.Is(err, ErrReadOnly) {
			t.Fatalf("read-only m4a Apply should return ErrReadOnly, got %v", err)
		}
	}
}

// TestM4A_ClearTags verifies that Apply with empty-string pointer values
// removes previously written tags (clear semantics per the Tags contract).
func TestM4A_ClearTags(t *testing.T) {
	_, writable := For("m4a")
	if !writable {
		t.Skip("m4a is read-only on this build; skipping clear test")
	}

	path := copyToTemp(t, "sample.m4a")
	w, _ := For("m4a")

	// Step 1: write real values so we have something to clear.
	if err := w.Apply(path, Tags{Title: strp("ClearMe"), Artist: strp("ArtistX")}, CoverKeep, nil); err != nil {
		t.Fatalf("Apply (set): %v", err)
	}

	// Step 2: confirm they were written.
	got, _, err := w.Read(path)
	if err != nil {
		t.Fatalf("Read after set: %v", err)
	}
	if got.Title == nil || *got.Title != "ClearMe" {
		t.Fatalf("pre-clear Title = %v, want ClearMe", got.Title)
	}
	if got.Artist == nil || *got.Artist != "ArtistX" {
		t.Fatalf("pre-clear Artist = %v, want ArtistX", got.Artist)
	}

	// Step 3: clear Title and Artist by passing non-nil empty-string pointers.
	if err := w.Apply(path, Tags{Title: strp(""), Artist: strp("")}, CoverKeep, nil); err != nil {
		t.Fatalf("Apply (clear): %v", err)
	}

	// Step 4: assert both fields are gone.
	got, _, err = w.Read(path)
	if err != nil {
		t.Fatalf("Read after clear: %v", err)
	}
	if got.Title != nil {
		t.Errorf("after clear: Title = %q, want nil", *got.Title)
	}
	if got.Artist != nil {
		t.Errorf("after clear: Artist = %q, want nil", *got.Artist)
	}
}
