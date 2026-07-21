package music

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ProbeAudio must extract duration and bitrate from an Ogg Vorbis file.
func TestProbeAudio_OggVorbis(t *testing.T) {
	requireFFmpeg(t)

	dir := t.TempDir()
	dst := filepath.Join(dir, "test.ogg")
	// 3 seconds so rounding noise cannot flip the integer result.
	out, err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-codec:a", "libvorbis", dst).CombinedOutput()
	if err != nil {
		// Fall back to ffmpeg's built-in (experimental) vorbis encoder; it
		// refuses mono input, hence the stereo upmix.
		out, err = exec.Command("ffmpeg", "-y",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
			"-ac", "2", "-codec:a", "vorbis", "-strict", "experimental", dst).CombinedOutput()
		if err != nil {
			t.Skipf("ffmpeg cannot encode vorbis: %v\n%s", err, out)
		}
	}

	dur, br := ProbeAudio(dst, "ogg")
	if dur != 3 {
		t.Errorf("duration = %d, want 3", dur)
	}
	if br <= 0 {
		t.Errorf("bitrate = %d, want > 0", br)
	}
}

// ProbeAudio must extract duration from an Ogg Opus file (also served with the
// .ogg extension). Opus granules always tick at 48 kHz regardless of the
// original sample rate.
func TestProbeAudio_OggOpus(t *testing.T) {
	requireFFmpeg(t)

	dir := t.TempDir()
	dst := filepath.Join(dir, "test-opus.ogg")
	out, err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-codec:a", "libopus", dst).CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg cannot encode opus: %v\n%s", err, out)
	}

	dur, _ := ProbeAudio(dst, "ogg")
	if dur != 3 {
		t.Errorf("duration = %d, want 3", dur)
	}
}

// Garbage bytes must not be mistaken for a valid Ogg stream.
func TestProbeAudio_OggGarbage(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "garbage.ogg")
	if err := os.WriteFile(dst, []byte("not really ogg at all"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	dur, br := ProbeAudio(dst, "ogg")
	if dur != 0 || br != 0 {
		t.Errorf("garbage probed as (%d, %d), want (0, 0)", dur, br)
	}
}
