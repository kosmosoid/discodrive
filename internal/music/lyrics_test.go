package music

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLRCSynced(t *testing.T) {
	in := "[00:12.50]Hello\n[01:05.00]World\n"
	lines, synced := ParseLRC(in)
	if !synced {
		t.Fatalf("expected synced=true")
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Start != 12500 || lines[0].Text != "Hello" {
		t.Errorf("line0=%+v, want start=12500 text=Hello", lines[0])
	}
	if lines[1].Start != 65000 || lines[1].Text != "World" {
		t.Errorf("line1=%+v, want start=65000 text=World", lines[1])
	}
}

func TestParseLRCPlain(t *testing.T) {
	in := "Hello\nWorld"
	lines, synced := ParseLRC(in)
	if synced {
		t.Fatalf("expected synced=false for plain text")
	}
	if len(lines) != 2 || lines[0].Text != "Hello" || lines[1].Text != "World" {
		t.Errorf("lines=%+v, want Hello/World unsynced", lines)
	}
}

func TestParseLRCEmpty(t *testing.T) {
	lines, synced := ParseLRC("")
	if synced || len(lines) != 0 {
		t.Errorf("empty input: lines=%+v synced=%v, want 0/false", lines, synced)
	}
}

func TestParseLRCMultiTimestamp(t *testing.T) {
	in := "[00:10.00][00:30.00]Chorus\n[00:20.00]Verse\n"
	lines, synced := ParseLRC(in)
	if !synced {
		t.Fatalf("expected synced=true")
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (chorus emitted twice)", len(lines))
	}
	// Expect sorted by Start: 10000 Chorus, 20000 Verse, 30000 Chorus
	want := []LyricLine{{Start: 10000, Text: "Chorus"}, {Start: 20000, Text: "Verse"}, {Start: 30000, Text: "Chorus"}}
	for i, w := range want {
		if lines[i].Start != w.Start || lines[i].Text != w.Text {
			t.Errorf("line%d=%+v, want %+v", i, lines[i], w)
		}
	}
}

func TestReadLyricsSidecar(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audio, []byte("not real audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "song.lrc"), []byte("[00:01.00]Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, synced := ReadLyrics(audio)
	if !synced {
		t.Errorf("synced=false, want true (sidecar present)")
	}
	if raw == "" {
		t.Errorf("raw empty, want sidecar content")
	}
}

func TestReadLyricsNone(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audio, []byte("not real audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := ReadLyrics(audio)
	if raw != "" {
		t.Errorf("raw=%q, want empty (no tags, no sidecar)", raw)
	}
}
