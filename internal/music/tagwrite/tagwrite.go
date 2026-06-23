// Package tagwrite writes audio metadata (tags + cover art) back to files,
// one implementation per format. Formats without a viable pure-Go writer are
// registered as read-only: Read/Cover work, Apply returns ErrReadOnly.
package tagwrite

import (
	"errors"

	"github.com/dhowden/tag"
)

// ErrReadOnly is returned by Apply for formats that can be read but not written.
var ErrReadOnly = errors.New("tagwrite: format is read-only")

// Tags holds editable text/number tags. A nil pointer means "leave unchanged";
// a non-nil pointer means "set this value" (empty string or 0 clears the tag).
type Tags struct {
	Title, Artist, Album, AlbumArtist, Genre *string
	Year, Track, Disc                        *int
}

// Cover is replacement cover-art image data.
type Cover struct {
	Data []byte
	Mime string
}

// CoverChange selects what happens to the embedded cover on Apply.
type CoverChange int

const (
	CoverKeep CoverChange = iota
	CoverRemove
	CoverReplace
)

// Writer reads and rewrites the metadata of one audio format.
type Writer interface {
	// Read returns current tags and whether a cover is embedded.
	Read(path string) (Tags, bool, error)
	// Apply rewrites the file at path in place with the given changes.
	Apply(path string, t Tags, cc CoverChange, cover *Cover) error
	// Cover returns embedded cover bytes and mime, or ok=false.
	Cover(path string) (data []byte, mime string, ok bool)
}

// registry maps lowercase extension (no dot) → writer. writable reports
// whether Apply is supported for that extension.
var registry = map[string]Writer{
	"mp3":  mp3Writer{},
	"flac": flacWriter{},
	"m4a":  mp4Writer{},
	"ogg":  oggWriter{},
}

var writable = map[string]bool{
	"mp3":  true,
	"flac": true,
	"m4a":  true,
	"ogg":  false, // read-only: pure-Go Ogg Vorbis comment writing requires bitstream re-pagination
}

// tagsFromDhowden converts a dhowden/tag Metadata value into Tags.
// Empty strings are treated as absent (nil pointer).
func tagsFromDhowden(m tag.Metadata) Tags {
	var t Tags
	if v := m.Title(); v != "" {
		t.Title = &v
	}
	if v := m.Artist(); v != "" {
		t.Artist = &v
	}
	if v := m.Album(); v != "" {
		t.Album = &v
	}
	if v := m.AlbumArtist(); v != "" {
		t.AlbumArtist = &v
	}
	if v := m.Genre(); v != "" {
		t.Genre = &v
	}
	if v := m.Year(); v != 0 {
		n := v
		t.Year = &n
	}
	if tr, _ := m.Track(); tr != 0 {
		n := tr
		t.Track = &n
	}
	if d, _ := m.Disc(); d != 0 {
		n := d
		t.Disc = &n
	}
	return t
}

// For returns the writer for ext and whether the format is writable.
func For(ext string) (Writer, bool) {
	w, ok := registry[ext]
	if !ok {
		return nil, false
	}
	return w, writable[ext]
}

// Writable reports whether Apply is supported for ext.
func Writable(ext string) bool { return writable[ext] }
