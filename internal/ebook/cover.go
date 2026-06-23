package ebook

import (
	"os"
	"path/filepath"
)

// mimeToExt maps common image MIME types to file extensions.
var mimeToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

// WriteCover caches cover image bytes to disk under
// <storageRoot>/.covers/ebooks/<bookID>.<ext>. The ext is derived from mimeType
// (defaults to "jpg" for unknown types). Returns the relative path from
// storageRoot (e.g. ".covers/ebooks/<id>.jpg") for storing in books.cover_path.
// The directory is created if missing. Existing files are overwritten.
func WriteCover(storageRoot, bookID string, data []byte, mimeType string) (string, error) {
	ext, ok := mimeToExt[mimeType]
	if !ok {
		ext = "jpg"
	}

	dir := filepath.Join(storageRoot, ".covers", "ebooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	filename := bookID + "." + ext
	dst := filepath.Join(dir, filename)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}

	return filepath.Join(".covers", "ebooks", filename), nil
}

// RemoveCover deletes a cached cover file at <storageRoot>/<relPath>.
// Returns nil if the file does not exist (best-effort semantics).
func RemoveCover(storageRoot, relPath string) error {
	if relPath == "" {
		return nil
	}
	err := os.Remove(filepath.Join(storageRoot, relPath))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
