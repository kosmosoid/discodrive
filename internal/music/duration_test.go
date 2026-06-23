package music

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestProbeAudioMP3 synthesizes a 3-second MP3 and verifies that ProbeAudio
// returns a duration close to 3 seconds and a non-zero bitrate.
func TestProbeAudioMP3(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH")
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "sine3.mp3")
	out, err := exec.Command("ffmpeg",
		"-y",
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=3",
		"-b:a", "128k",
		dst,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	dur, br := ProbeAudio(dst, "mp3")
	if dur < 2 || dur > 4 {
		t.Errorf("MP3 duration = %d, want 2–4 (≈3)", dur)
	}
	if br <= 0 {
		t.Errorf("MP3 bitrate = %d, want > 0", br)
	}
}

// TestProbeAudioFLAC synthesizes a 3-second FLAC and verifies that ProbeAudio
// returns a duration close to 3 seconds and a non-zero bitrate.
func TestProbeAudioFLAC(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH")
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "sine3.flac")
	out, err := exec.Command("ffmpeg",
		"-y",
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=3",
		dst,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	dur, br := ProbeAudio(dst, "flac")
	if dur < 2 || dur > 4 {
		t.Errorf("FLAC duration = %d, want 2–4 (≈3)", dur)
	}
	if br <= 0 {
		t.Errorf("FLAC bitrate = %d, want > 0", br)
	}
}

// TestProbeAudioMissing verifies that a missing file returns (0, 0).
func TestProbeAudioMissing(t *testing.T) {
	dur, br := ProbeAudio("nonexistent.mp3", "mp3")
	if dur != 0 || br != 0 {
		t.Errorf("missing file: got (%d, %d), want (0, 0)", dur, br)
	}
}

// TestProbeAudioUnknownSuffix verifies that an unsupported format returns (0, 0).
func TestProbeAudioUnknownSuffix(t *testing.T) {
	dur, br := ProbeAudio("audio.m4a", "m4a")
	if dur != 0 || br != 0 {
		t.Errorf("unknown suffix: got (%d, %d), want (0, 0)", dur, br)
	}
}
