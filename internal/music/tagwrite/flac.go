package tagwrite

import (
	"strconv"

	flac "github.com/go-flac/go-flac/v2"
	"github.com/go-flac/flacpicture/v2"
	"github.com/go-flac/flacvorbis/v2"
)

type flacWriter struct{}

// findComment returns the VorbisComment block and its index, or a new empty
// block with index -1 if none exists.
func findComment(f *flac.File) (*flacvorbis.MetaDataBlockVorbisComment, int) {
	for i, m := range f.Meta {
		if m.Type == flac.VorbisComment {
			vc, err := flacvorbis.ParseFromMetaDataBlock(*m)
			if err == nil {
				return vc, i
			}
		}
	}
	return flacvorbis.New(), -1
}

func firstComment(vc *flacvorbis.MetaDataBlockVorbisComment, key string) string {
	vals, err := vc.Get(key)
	if err != nil || len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (flacWriter) Read(path string) (Tags, bool, error) {
	f, err := flac.ParseFile(path)
	if err != nil {
		return Tags{}, false, err
	}
	vc, _ := findComment(f)

	var t Tags
	assign := func(key string, dst **string) {
		if v := firstComment(vc, key); v != "" {
			vv := v
			*dst = &vv
		}
	}
	assign("TITLE", &t.Title)
	assign("ARTIST", &t.Artist)
	assign("ALBUM", &t.Album)
	assign("ALBUMARTIST", &t.AlbumArtist)
	assign("GENRE", &t.Genre)
	if v := firstComment(vc, "DATE"); v != "" {
		if n := leadingInt(v); n > 0 {
			t.Year = &n
		}
	}
	if v := firstComment(vc, "TRACKNUMBER"); v != "" {
		if n := leadingInt(v); n > 0 {
			t.Track = &n
		}
	}
	if v := firstComment(vc, "DISCNUMBER"); v != "" {
		if n := leadingInt(v); n > 0 {
			t.Disc = &n
		}
	}

	hasCover := false
	for _, m := range f.Meta {
		if m.Type == flac.Picture {
			hasCover = true
			break
		}
	}
	return t, hasCover, nil
}

func (flacWriter) Apply(path string, t Tags, cc CoverChange, cover *Cover) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return err
	}
	vc, idx := findComment(f)

	setKey := func(key string, p *string) {
		if p == nil {
			return
		}
		dropKey(vc, key)
		if *p != "" {
			_ = vc.Add(key, *p)
		}
	}
	setInt := func(key string, p *int) {
		if p == nil {
			return
		}
		dropKey(vc, key)
		if *p > 0 {
			_ = vc.Add(key, strconv.Itoa(*p))
		}
	}
	setKey("TITLE", t.Title)
	setKey("ARTIST", t.Artist)
	setKey("ALBUM", t.Album)
	setKey("ALBUMARTIST", t.AlbumArtist)
	setKey("GENRE", t.Genre)
	setInt("DATE", t.Year)
	setInt("TRACKNUMBER", t.Track)
	setInt("DISCNUMBER", t.Disc)

	cmtBlock := vc.Marshal()
	if idx >= 0 {
		f.Meta[idx] = &cmtBlock
	} else {
		f.Meta = append(f.Meta, &cmtBlock)
	}

	if cc == CoverRemove || cc == CoverReplace {
		// drop existing pictures
		kept := f.Meta[:0]
		for _, m := range f.Meta {
			if m.Type != flac.Picture {
				kept = append(kept, m)
			}
		}
		f.Meta = kept
		if cc == CoverReplace && cover != nil {
			pic, perr := flacpicture.NewFromImageData(
				flacpicture.PictureTypeFrontCover, "Front cover", cover.Data, cover.Mime)
			if perr != nil {
				return perr
			}
			pb := pic.Marshal()
			f.Meta = append(f.Meta, &pb)
		}
	}

	return f.Save(path)
}

func (flacWriter) Cover(path string) ([]byte, string, bool) {
	f, err := flac.ParseFile(path)
	if err != nil {
		return nil, "", false
	}
	for _, m := range f.Meta {
		if m.Type == flac.Picture {
			pic, perr := flacpicture.ParseFromMetaDataBlock(*m)
			if perr == nil && len(pic.ImageData) > 0 {
				return pic.ImageData, pic.MIME, true
			}
		}
	}
	return nil, "", false
}

// dropKey removes all values for key from the comment block (case-insensitive).
func dropKey(vc *flacvorbis.MetaDataBlockVorbisComment, key string) {
	var kept []string
	for _, c := range vc.Comments {
		if !hasPrefixFold(c, key+"=") {
			kept = append(kept, c)
		}
	}
	vc.Comments = kept
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a, b := s[i], prefix[i]
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}
