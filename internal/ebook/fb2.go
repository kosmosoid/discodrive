package ebook

import (
	"archive/zip"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

// parseFB2 reads metadata from a FictionBook 2 (.fb2) file.
// The FB2 format is a single XML document; all metadata lives under
// <description><title-info>.
func parseFB2(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	return parseFB2Bytes(data)
}

// parseFB2Zip reads the first *.fb2 entry from a zip archive and parses it.
func parseFB2Zip(path string) (Meta, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Meta{}, err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".fb2") {
			rc, err := f.Open()
			if err != nil {
				return Meta{}, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return Meta{}, err
			}
			return parseFB2Bytes(data)
		}
	}
	return Meta{}, errors.New("fb2: no .fb2 entry found in zip")
}

// --- FB2 XML structures ---

// fb2Book is the root <FictionBook> element.
type fb2Book struct {
	Description fb2Description `xml:"description"`
	Binaries    []fb2Binary    `xml:"binary"`
}

// fb2Description wraps <description>.
type fb2Description struct {
	TitleInfo fb2TitleInfo `xml:"title-info"`
}

// fb2TitleInfo maps <title-info> inside <description>.
type fb2TitleInfo struct {
	BookTitle  string      `xml:"book-title"`
	Authors    []fb2Author `xml:"author"`
	Lang       string      `xml:"lang"`
	Sequences  []fb2Seq    `xml:"sequence"`
	Genres     []string    `xml:"genre"`
	Annotation fb2Annot    `xml:"annotation"`
	Date       string      `xml:"date"`
	Coverpage  fb2Cover    `xml:"coverpage"`
}

// fb2Author maps a single <author> element.
// FB2 authors can have first/middle/last names or just a nickname.
type fb2Author struct {
	FirstName  string `xml:"first-name"`
	MiddleName string `xml:"middle-name"`
	LastName   string `xml:"last-name"`
	Nickname   string `xml:"nickname"`
}

// fb2Seq maps a <sequence> element (series name and number).
type fb2Seq struct {
	Name   string `xml:"name,attr"`
	Number string `xml:"number,attr"`
}

// fb2Annot is <annotation>; we capture its inner text via the chardata trick.
// Since annotation can contain child tags (<p>, <emphasis>, etc.) we accumulate
// all character data within the element.
type fb2Annot struct {
	Text string `xml:",innerxml"`
}

// fb2Cover maps <coverpage>.
type fb2Cover struct {
	Image fb2Image `xml:"image"`
}

// fb2Image maps <image> inside <coverpage>.
// The href is in the XLink namespace (xmlns:l="http://www.w3.org/1999/xlink").
// We bind both common prefixes (l: and xlink:) via the namespace URI.
type fb2Image struct {
	// XLink href attribute — must use the namespace URI, not the prefix.
	Href string `xml:"http://www.w3.org/1999/xlink href,attr"`
}

// fb2Binary maps a root-level <binary> element (embedded images, etc.).
type fb2Binary struct {
	ID          string `xml:"id,attr"`
	ContentType string `xml:"content-type,attr"`
	Data        string `xml:",chardata"` // base64-encoded content
}

// parseFB2Bytes parses FB2 metadata from raw XML bytes.
func parseFB2Bytes(data []byte) (Meta, error) {
	var book fb2Book
	if err := xml.Unmarshal(data, &book); err != nil {
		return Meta{}, err
	}

	ti := &book.Description.TitleInfo
	var m Meta

	// Title.
	m.Title = strings.TrimSpace(ti.BookTitle)

	// Authors.
	for _, a := range ti.Authors {
		name, sortName := fb2AuthorNames(a)
		if name == "" {
			continue
		}
		m.Authors = append(m.Authors, Author{Name: name, SortName: sortName})
	}

	// Language.
	m.Language = strings.TrimSpace(ti.Lang)

	// Series: take the first <sequence> element.
	if len(ti.Sequences) > 0 {
		seq := ti.Sequences[0]
		m.Series = strings.TrimSpace(seq.Name)
		if n, err := strconv.ParseFloat(strings.TrimSpace(seq.Number), 64); err == nil {
			m.SeriesIndex = n
		}
	}

	// Tags from <genre>.
	for _, g := range ti.Genres {
		if v := strings.TrimSpace(g); v != "" {
			m.Tags = append(m.Tags, v)
		}
	}

	// Description: strip XML tags from annotation's inner XML.
	if raw := strings.TrimSpace(ti.Annotation.Text); raw != "" {
		m.Description = stripXMLTags(raw)
	}

	// Date.
	m.Date = strings.TrimSpace(ti.Date)

	// Cover: resolve href="#ID" → strip "#" → find matching <binary>.
	coverID := strings.TrimPrefix(ti.Coverpage.Image.Href, "#")
	if coverID != "" {
		for _, bin := range book.Binaries {
			if bin.ID == coverID {
				decoded, err := base64.StdEncoding.DecodeString(
					strings.TrimSpace(bin.Data),
				)
				if err == nil && len(decoded) > 0 {
					m.CoverData = decoded
					m.CoverType = bin.ContentType
				}
				break
			}
		}
	}

	return m, nil
}

// fb2AuthorNames builds the display name and sort name for an FB2 author.
// If the author has a last name, sort order is "last first[middle]" (lowercased).
// If only a nickname is present, both Name and SortName derive from it.
func fb2AuthorNames(a fb2Author) (name, sortName string) {
	first := strings.TrimSpace(a.FirstName)
	middle := strings.TrimSpace(a.MiddleName)
	last := strings.TrimSpace(a.LastName)
	nick := strings.TrimSpace(a.Nickname)

	if first == "" && last == "" {
		// Nickname-only author.
		if nick == "" {
			return "", ""
		}
		return nick, strings.ToLower(nick)
	}

	// Build display name: "First [Middle] Last"
	parts := []string{}
	given := strings.TrimSpace(first + " " + middle)
	if given != "" {
		parts = append(parts, given)
	}
	if last != "" {
		parts = append(parts, last)
	}
	name = strings.Join(parts, " ")

	// Sort name: "last first[middle]" lowercased.
	sortParts := []string{}
	if last != "" {
		sortParts = append(sortParts, last)
	}
	if given != "" {
		sortParts = append(sortParts, given)
	}
	sortName = strings.ToLower(strings.Join(sortParts, " "))

	return name, sortName
}

// stripXMLTags removes XML/HTML tags from a string, leaving only text content.
func stripXMLTags(s string) string {
	// Decode the inner XML by re-parsing it as a fragment.
	// Wrap in a root element so xml.Decoder can process it.
	dec := xml.NewDecoder(strings.NewReader("<r>" + s + "</r>"))
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			b.Write(cd)
		}
	}
	return strings.TrimSpace(b.String())
}
