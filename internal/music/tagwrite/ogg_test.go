package tagwrite

import (
	"errors"
	"testing"
)

func TestOGG_RoundTripOrReadOnly(t *testing.T) {
	path := copyToTemp(t, "sample.ogg")
	w, isWritable := For("ogg")
	if w == nil {
		t.Fatal("ogg must at least be readable")
	}
	// Read must always work.
	if _, _, err := w.Read(path); err != nil {
		t.Fatalf("Read: %v", err)
	}
	err := w.Apply(path, Tags{Title: strp("X")}, CoverKeep, nil)
	if isWritable {
		if err != nil {
			t.Fatalf("writable ogg Apply: %v", err)
		}
		got, _, _ := w.Read(path)
		if got.Title == nil || *got.Title != "X" {
			t.Errorf("Title = %v, want X", got.Title)
		}
	} else {
		if !errors.Is(err, ErrReadOnly) {
			t.Fatalf("read-only ogg Apply should return ErrReadOnly, got %v", err)
		}
	}
}
