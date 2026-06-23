package tagwrite

import "testing"

func TestFLAC_WriteThenReadRoundTrip(t *testing.T) {
	path := copyToTemp(t, "sample.flac")
	w, ok := For("flac")
	if !ok {
		t.Fatal("flac must be writable")
	}
	if err := w.Apply(path, Tags{
		Title:  strp("Flac Title"),
		Artist: strp("Flac Artist"),
		Album:  strp("Flac Album"),
		Year:   intp(2019),
		Track:  intp(5),
	}, CoverKeep, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _, err := w.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Title == nil || *got.Title != "Flac Title" {
		t.Errorf("Title = %v, want Flac Title", got.Title)
	}
	if got.Track == nil || *got.Track != 5 {
		t.Errorf("Track = %v, want 5", got.Track)
	}
}
