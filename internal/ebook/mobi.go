package ebook

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"strings"
)

// parseMOBI reads metadata from a MOBI, AZW, or AZW3 file.
// It parses the PalmDB container, MOBI header, and EXTH record block.
// Cover extraction is best-effort: any bounds error silently leaves CoverData nil.
func parseMOBI(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	return parseMOBIBytes(data)
}

// parseMOBIBytes is the testable core that operates on an in-memory buffer.
func parseMOBIBytes(data []byte) (Meta, error) {
	// --- PalmDB header ---
	// Minimum size: 78 bytes header + at least 1 record-info entry (8 bytes) = 86 bytes.
	if len(data) < 86 {
		return Meta{}, errors.New("mobi: file too short to be a valid MOBI")
	}

	// Number of records is a big-endian uint16 at offset 76.
	numRecords := int(beU16(data, 76))
	if numRecords < 1 {
		return Meta{}, errors.New("mobi: no records in PalmDB")
	}

	// Record-info list starts at offset 78; each entry is 8 bytes.
	// Entry layout: uint32 offset, uint8 attribs, uint8 uniqueID[3].
	recInfoBase := 78
	recInfoSize := 8

	// We need at least record 0's info (and record 1's info to bound record 0).
	minNeeded := recInfoBase + (numRecords)*recInfoSize
	if len(data) < minNeeded {
		return Meta{}, errors.New("mobi: truncated record-info list")
	}

	// Record 0 offset.
	rec0Offset := int(beU32(data, recInfoBase))
	if rec0Offset < 0 || rec0Offset >= len(data) {
		return Meta{}, errors.New("mobi: record 0 offset out of bounds")
	}

	// Record 0 length: use record 1's offset as the upper bound.
	var rec0End int
	if numRecords > 1 {
		rec0End = int(beU32(data, recInfoBase+recInfoSize))
	} else {
		rec0End = len(data)
	}
	if rec0End < rec0Offset || rec0End > len(data) {
		return Meta{}, errors.New("mobi: record 0 bounds invalid")
	}
	rec0 := data[rec0Offset:rec0End]

	// --- PalmDOC header (16 bytes at start of record 0) ---
	if len(rec0) < 16 {
		return Meta{}, errors.New("mobi: record 0 too short for PalmDOC header")
	}
	// encryptionType at PalmDOC+4 (uint16). Value 2 = DRM-encrypted; we still
	// attempt to read the MOBI header metadata even if content is encrypted.

	// --- MOBI header (immediately after PalmDOC header at rec0[16:]) ---
	mobiOff := 16
	if len(rec0) < mobiOff+8 {
		return Meta{}, errors.New("mobi: record 0 too short for MOBI header")
	}

	// Verify "MOBI" magic.
	if string(rec0[mobiOff:mobiOff+4]) != "MOBI" {
		return Meta{}, errors.New("mobi: MOBI identifier not found")
	}

	// MOBI header length at mobiOff+4 (uint32, big-endian).
	mobiHeaderLen := int(beU32(rec0, mobiOff+4))
	if mobiHeaderLen < 0x80+4 { // we need at least up to EXTH flags field
		return Meta{}, errors.New("mobi: MOBI header too short")
	}

	mobiEnd := mobiOff + mobiHeaderLen
	if mobiEnd > len(rec0) {
		return Meta{}, errors.New("mobi: MOBI header extends beyond record 0")
	}

	// Full Name (book title): offsets relative to record 0 start.
	// fullNameOffset at mobiOff+0x54 (84), fullNameLength at mobiOff+0x58 (88).
	if mobiOff+0x5C > len(rec0) {
		return Meta{}, errors.New("mobi: MOBI header too short for full-name fields")
	}
	fullNameOff := int(beU32(rec0, mobiOff+0x54))
	fullNameLen := int(beU32(rec0, mobiOff+0x58))

	var title string
	if fullNameOff >= 0 && fullNameLen > 0 &&
		fullNameOff+fullNameLen <= len(rec0) {
		title = string(rec0[fullNameOff : fullNameOff+fullNameLen])
	}

	// EXTH flags at mobiOff+0x80 (128), bit 0x40 = EXTH present.
	exthPresent := false
	if mobiOff+0x80+4 <= len(rec0) {
		exthFlags := beU32(rec0, mobiOff+0x80)
		exthPresent = (exthFlags & 0x40) != 0
	}

	// firstImageIndex at mobiOff+0x6C (108).
	firstImageIndex := uint32(0xFFFFFFFF) // sentinel: unknown
	if mobiOff+0x70 <= len(rec0) {
		firstImageIndex = beU32(rec0, mobiOff+0x6C)
	}

	// --- EXTH header ---
	type exthRecord struct {
		typ  uint32
		data []byte
	}
	var exthRecords []exthRecord

	if exthPresent {
		exthBase := mobiEnd // EXTH starts right after the MOBI header
		if exthBase+12 <= len(rec0) &&
			string(rec0[exthBase:exthBase+4]) == "EXTH" {

			// exthHeaderLen := beU32(rec0, exthBase+4) // total EXTH block length
			exthCount := int(beU32(rec0, exthBase+8))
			pos := exthBase + 12

			for i := 0; i < exthCount && pos+8 <= len(rec0); i++ {
				recType := beU32(rec0, pos)
				recLen := int(beU32(rec0, pos+4))
				if recLen < 8 || pos+recLen > len(rec0) {
					break // malformed record; stop parsing EXTH
				}
				recData := rec0[pos+8 : pos+recLen]
				exthRecords = append(exthRecords, exthRecord{typ: recType, data: recData})
				pos += recLen
			}
		}
	}

	// --- Build Meta from parsed fields ---
	var m Meta

	// Title: prefer MOBI Full Name; fall back to EXTH 503.
	m.Title = strings.TrimSpace(title)

	// Gather EXTH values.
	var coverOffsetVal uint32
	hasCoverOffset := false

	for _, r := range exthRecords {
		switch r.typ {
		case 100: // author
			name := strings.TrimSpace(string(r.data))
			if name != "" {
				m.Authors = append(m.Authors, Author{
					Name:     name,
					SortName: strings.ToLower(name),
				})
			}
		case 101: // publisher
			if m.Publisher == "" {
				m.Publisher = strings.TrimSpace(string(r.data))
			}
		case 103: // description
			if m.Description == "" {
				m.Description = strings.TrimSpace(string(r.data))
			}
		case 104: // isbn
			if m.ISBN == "" {
				m.ISBN = strings.TrimSpace(string(r.data))
			}
		case 105: // subject → Tags
			tag := strings.TrimSpace(string(r.data))
			if tag != "" {
				m.Tags = append(m.Tags, tag)
			}
		case 106: // publishing date
			if m.Date == "" {
				m.Date = strings.TrimSpace(string(r.data))
			}
		case 201: // cover offset (uint32 index relative to firstImageIndex)
			if len(r.data) == 4 {
				coverOffsetVal = binary.BigEndian.Uint32(r.data)
				hasCoverOffset = true
			}
		case 503: // updated title
			if m.Title == "" {
				m.Title = strings.TrimSpace(string(r.data))
			}
		}
	}

	// Cover (best-effort): locate the image record and read its bytes.
	if hasCoverOffset && firstImageIndex != 0xFFFFFFFF {
		coverRecIdx := int(firstImageIndex) + int(coverOffsetVal)
		if coverRecIdx >= 0 && coverRecIdx < numRecords {
			coverData := mobiReadRecord(data, recInfoBase, recInfoSize, coverRecIdx, numRecords)
			if len(coverData) > 0 {
				m.CoverData = coverData
				m.CoverType = sniffImageType(coverData)
			}
		}
	}

	return m, nil
}

// mobiReadRecord extracts the raw bytes of PalmDB record at index idx.
// Returns nil on any bounds error (cover is best-effort).
func mobiReadRecord(data []byte, recInfoBase, recInfoSize, idx, numRecords int) []byte {
	entryOff := recInfoBase + idx*recInfoSize
	if entryOff+4 > len(data) {
		return nil
	}
	start := int(beU32(data, entryOff))
	var end int
	if idx+1 < numRecords {
		nextOff := entryOff + recInfoSize
		if nextOff+4 > len(data) {
			return nil
		}
		end = int(beU32(data, nextOff))
	} else {
		end = len(data)
	}
	if start < 0 || end < start || end > len(data) {
		return nil
	}
	return data[start:end]
}

// sniffImageType returns the MIME type of an image based on magic bytes.
// Returns an empty string for unknown formats.
func sniffImageType(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return "image/jpeg"
	}
	if len(b) >= 4 && bytes.Equal(b[:4], []byte{0x89, 0x50, 0x4E, 0x47}) {
		return "image/png"
	}
	if len(b) >= 3 && string(b[:3]) == "GIF" {
		return "image/gif"
	}
	return ""
}

// beU16 reads a big-endian uint16 from b at offset off.
// Callers must ensure len(b) >= off+2 before calling.
func beU16(b []byte, off int) uint16 {
	return binary.BigEndian.Uint16(b[off : off+2])
}

// beU32 reads a big-endian uint32 from b at offset off.
// Callers must ensure len(b) >= off+4 before calling.
func beU32(b []byte, off int) uint32 {
	return binary.BigEndian.Uint32(b[off : off+4])
}
