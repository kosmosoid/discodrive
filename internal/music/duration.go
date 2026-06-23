package music

import (
	"io"
	"math"
	"os"

	"github.com/mewkiz/flac"
	"github.com/tcolgate/mp3"
)

// ProbeAudio returns the duration (seconds) and average bitrate (kbps) of an
// audio file, or (0, 0) if it cannot be determined. Pure Go; dispatches by
// lowercase suffix ("mp3", "flac"). Other formats return (0, 0) for now.
func ProbeAudio(path, suffix string) (durationSec int, bitrateKbps int) {
	switch suffix {
	case "mp3":
		return probeMP3(path)
	case "flac":
		return probeFLAC(path)
	default:
		return 0, 0
	}
}

// probeMP3 iterates all MP3 frames to sum their durations, then derives average
// bitrate from file size. Handles both CBR and VBR correctly.
func probeMP3(path string) (durationSec int, bitrateKbps int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	d := mp3.NewDecoder(f)
	var totalNs float64
	var frame mp3.Frame
	var skipped int
	for {
		err := d.Decode(&frame, &skipped)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Non-fatal frame error: skip and continue.
			continue
		}
		totalNs += frame.Duration().Seconds()
	}

	if totalNs <= 0 {
		return 0, 0
	}

	durationSec = int(math.Round(totalNs))
	if durationSec == 0 {
		durationSec = 1
	}

	if fi, err := os.Stat(path); err == nil {
		bitrateKbps = int(fi.Size() * 8 / int64(durationSec) / 1000)
	}
	return durationSec, bitrateKbps
}

// probeFLAC reads the FLAC StreamInfo header to derive duration without
// decoding audio frames.
func probeFLAC(path string) (durationSec int, bitrateKbps int) {
	stream, err := flac.ParseFile(path)
	if err != nil {
		return 0, 0
	}
	defer stream.Close()

	info := stream.Info
	if info == nil || info.SampleRate == 0 {
		return 0, 0
	}

	durationSec = int(info.NSamples / uint64(info.SampleRate))
	if durationSec == 0 {
		return 0, 0
	}

	if fi, err := os.Stat(path); err == nil {
		bitrateKbps = int(fi.Size() * 8 / int64(durationSec) / 1000)
	}
	return durationSec, bitrateKbps
}
