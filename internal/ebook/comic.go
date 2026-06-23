package ebook

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nwaples/rardecode"
)

// comicInfo maps the root <ComicInfo> element found in ComicInfo.xml files
// embedded in CBZ/CBR archives (the ComicRack metadata standard).
type comicInfo struct {
	Title       string `xml:"Title"`
	Series      string `xml:"Series"`
	Number      string `xml:"Number"`
	Writer      string `xml:"Writer"`
	Genre       string `xml:"Genre"`
	Summary     string `xml:"Summary"`
	LanguageISO string `xml:"LanguageISO"`
}

// comicImageExts is the set of supported image extensions for cover selection.
var comicImageExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// maxComicEntryBytes caps how many bytes we buffer from a single comic archive
// entry (cover image or ComicInfo.xml), bounding memory against decompression
// bombs in untrusted uploads. NOTE: for CBR this does not fully neutralize
// GO-2025-4020 — rardecode allocates its decode window from the archive header
// before entry bytes are read; that residual DoS is tracked in SECURITY_AUDIT.md
// until rardecode is replaced.
const maxComicEntryBytes = 64 << 20 // 64 MiB — generous for a single comic page

// readComicEntry reads up to maxComicEntryBytes from r. ok is false if the read
// failed or the entry exceeded the cap (in which case the caller skips it rather
// than buffering an unbounded amount).
func readComicEntry(r io.Reader) (data []byte, ok bool) {
	data, err := io.ReadAll(io.LimitReader(r, maxComicEntryBytes+1))
	if err != nil || int64(len(data)) > maxComicEntryBytes {
		return nil, false
	}
	return data, true
}

// parseComic reads metadata and cover from a CBZ or CBR file.
// CBZ is a ZIP archive; CBR is a RAR archive. Both may contain a
// ComicInfo.xml with structured metadata (ComicRack standard). The cover
// is the lexicographically first image entry in the archive.
func parseComic(path string) (Meta, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".cbz":
		return parseCBZ(path)
	case ".cbr":
		return parseCBR(path)
	default:
		return Meta{}, &unsupportedComicExt{ext: ext}
	}
}

type unsupportedComicExt struct{ ext string }

func (e *unsupportedComicExt) Error() string {
	return "comic: unsupported extension: " + e.ext
}

// parseCBZ reads a CBZ (zip) comic archive.
func parseCBZ(path string) (Meta, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Meta{}, err
	}
	defer zr.Close()

	// Collect image entries and look for ComicInfo.xml in one pass.
	var (
		infoData  []byte
		imageKeys []string                // sorted names of image entries
		imageMap  = map[string]*zip.File{} // name → zip.File
	)

	for _, f := range zr.File {
		name := f.Name
		lower := strings.ToLower(name)
		if strings.EqualFold(filepath.Base(lower), "comicinfo.xml") {
			rc, err := f.Open()
			if err != nil {
				return Meta{}, err
			}
			infoData, _ = readComicEntry(rc)
			rc.Close()
			continue
		}
		if _, ok := comicImageExts[filepath.Ext(lower)]; ok {
			imageKeys = append(imageKeys, name)
			imageMap[name] = f
		}
	}

	m := comicBuildMeta(infoData)

	// Cover: lexicographically first image.
	if len(imageKeys) > 0 {
		sort.Strings(imageKeys)
		f := imageMap[imageKeys[0]]
		rc, err := f.Open()
		if err == nil {
			data, ok := readComicEntry(rc)
			rc.Close()
			if ok && len(data) > 0 {
				m.CoverData = data
				m.CoverType = comicImageExts[filepath.Ext(strings.ToLower(imageKeys[0]))]
			}
		}
	}

	return m, nil
}

// parseCBR reads a CBR (rar) comic archive using the rardecode streaming reader.
// Since RAR is a sequential format with no random-access API, we make a single
// pass collecting ComicInfo.xml and all image entries, then pick the cover.
func parseCBR(path string) (Meta, error) {
	rr, err := rardecode.OpenReader(path, "")
	if err != nil {
		return Meta{}, err
	}
	defer rr.Close()

	type imageEntry struct {
		name string
		data []byte
	}

	var (
		infoData []byte
		images   []imageEntry
	)

	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Meta{}, err
		}

		lower := strings.ToLower(hdr.Name)
		base := strings.ToLower(filepath.Base(lower))

		if base == "comicinfo.xml" {
			infoData, _ = readComicEntry(rr)
			continue
		}
		if _, ok := comicImageExts[filepath.Ext(lower)]; ok {
			if data, ok := readComicEntry(rr); ok {
				images = append(images, imageEntry{name: hdr.Name, data: data})
			}
		}
	}

	m := comicBuildMeta(infoData)

	// Cover: lexicographically first image by name.
	if len(images) > 0 {
		sort.Slice(images, func(i, j int) bool {
			return images[i].name < images[j].name
		})
		first := images[0]
		m.CoverData = first.data
		m.CoverType = comicImageExts[filepath.Ext(strings.ToLower(first.name))]
	}

	return m, nil
}

// comicBuildMeta parses ComicInfo.xml bytes (if non-nil) into a Meta.
// Fields not present in the XML remain zero-valued; ReadMeta fills the
// title fallback from the filename after this returns.
func comicBuildMeta(xmlData []byte) Meta {
	if len(xmlData) == 0 {
		return Meta{}
	}

	var info comicInfo
	if err := xml.Unmarshal(xmlData, &info); err != nil {
		// Malformed ComicInfo.xml: return empty metadata rather than an error
		// so the caller still gets a valid (sparse) Meta with a cover image.
		return Meta{}
	}

	var m Meta
	m.Title = strings.TrimSpace(info.Title)
	m.Series = strings.TrimSpace(info.Series)
	m.Description = strings.TrimSpace(info.Summary)
	m.Language = strings.TrimSpace(info.LanguageISO)

	// Number may be "2" or "2.5".
	if n := strings.TrimSpace(info.Number); n != "" {
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			m.SeriesIndex = f
		}
	}

	// Writer: comma-separated list of author names.
	for _, name := range splitComicList(info.Writer) {
		m.Authors = append(m.Authors, Author{
			Name:     name,
			SortName: comicSortName(name),
		})
	}

	// Genre: comma-separated list of tags.
	m.Tags = splitComicList(info.Genre)

	return m
}

// splitComicList splits a comma-separated ComicInfo field (Writer, Genre, etc.)
// into trimmed, non-empty strings.
func splitComicList(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// comicSortName produces a sort name for a comic author name.
// "Alan Moore" → "moore alan"; single tokens stay lowercased as-is.
func comicSortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return strings.ToLower(name)
	}
	// Move last name to front: "Alan Moore" → "moore alan"
	last := parts[len(parts)-1]
	rest := parts[:len(parts)-1]
	return strings.ToLower(last + " " + strings.Join(rest, " "))
}
