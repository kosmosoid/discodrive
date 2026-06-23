// Package storage implements "files as files on disk" — a mirror of the user's
// tree on the local filesystem. Cures the Seafile trauma:
// service dies → you open the folder and the files are still there.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiskEntry is a single entry in the on-disk tree (relative path from the root).
type DiskEntry struct {
	Rel   string
	IsDir bool
}

// ErrPathEscape is returned when a relative path attempts to escape the storage root.
var ErrPathEscape = errors.New("path escapes storage root")

// Storage abstracts the file store (designed to support future S3 backends).
type Storage interface {
	// WriteFile writes content at the given relative path (creating parents),
	// and returns the size and sha256 hex digest.
	WriteFile(rel string, r io.Reader) (size int64, sha256hex string, err error)
	// Mkdir creates the directory and any missing parents.
	Mkdir(rel string) error
	// Move renames/moves a path (file or directory).
	Move(oldRel, newRel string) error
	// Copy copies a file src→dst (creating dst parents). Used for version snapshots.
	Copy(srcRel, dstRel string) error
	// Append appends data to the end of a file (creates it if absent). Used for chunks.
	Append(rel string, r io.Reader) error
	// Remove deletes a path recursively (used by GC; not called on soft-delete).
	Remove(rel string) error
	// Open opens a file for reading.
	Open(rel string) (*os.File, error)
	// AbsPath returns the absolute path (for X-Accel / direct reads).
	AbsPath(rel string) (string, error)
	// Walk returns the subtree under rel (parents before children), with relative paths.
	Walk(rel string) ([]DiskEntry, error)
}

// LocalDisk is a Storage implementation backed by the local filesystem rooted at root.
type LocalDisk struct {
	root string
}

func NewLocalDisk(root string) *LocalDisk {
	return &LocalDisk{root: filepath.Clean(root)}
}

// abs safely joins the root and relative path, preventing path escape.
func (d *LocalDisk) abs(rel string) (string, error) {
	clean := filepath.Clean("/" + strings.ReplaceAll(rel, "\\", "/"))
	full := filepath.Join(d.root, clean)
	if full != d.root && !strings.HasPrefix(full, d.root+string(os.PathSeparator)) {
		return "", ErrPathEscape
	}
	return full, nil
}

func (d *LocalDisk) WriteFile(rel string, r io.Reader) (int64, string, error) {
	full, err := d.abs(rel)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, "", err
	}
	f, err := os.Create(full)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return 0, "", err
	}
	if err := f.Sync(); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (d *LocalDisk) Mkdir(rel string) error {
	full, err := d.abs(rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, 0o755)
}

func (d *LocalDisk) Move(oldRel, newRel string) error {
	oldFull, err := d.abs(oldRel)
	if err != nil {
		return err
	}
	newFull, err := d.abs(newRel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
		return err
	}
	return os.Rename(oldFull, newFull)
}

func (d *LocalDisk) Copy(srcRel, dstRel string) error {
	srcFull, err := d.abs(srcRel)
	if err != nil {
		return err
	}
	dstFull, err := d.abs(dstRel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstFull), 0o755); err != nil {
		return err
	}
	in, err := os.Open(srcFull)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dstFull)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func (d *LocalDisk) Append(rel string, r io.Reader) error {
	full, err := d.abs(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return f.Sync()
}

func (d *LocalDisk) Remove(rel string) error {
	full, err := d.abs(rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(full)
}

func (d *LocalDisk) Open(rel string) (*os.File, error) {
	full, err := d.abs(rel)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func (d *LocalDisk) AbsPath(rel string) (string, error) {
	return d.abs(rel)
}

func (d *LocalDisk) Walk(rel string) ([]DiskEntry, error) {
	full, err := d.abs(rel)
	if err != nil {
		return nil, err
	}
	var out []DiskEntry
	walkErr := filepath.WalkDir(full, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if p == full {
			return nil // skip the root directory itself
		}
		r, err := filepath.Rel(d.root, p)
		if err != nil {
			return err
		}
		out = append(out, DiskEntry{Rel: filepath.ToSlash(r), IsDir: e.IsDir()})
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return nil, walkErr
	}
	return out, nil
}
