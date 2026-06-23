package webdav

import (
	"context"
	"io/fs"
	"os"
	"path"
	"strings"

	"golang.org/x/net/webdav"

	"discodrive/internal/db"
	"discodrive/internal/storage"
)

// ctxKey holds context keys populated by the Auth middleware (see auth.go, Task 7).
type ctxKey int

const (
	ctxUserKey ctxKey = iota
	ctxDeviceKey
)

// FileSystem is a webdav.FileSystem backed by the sync core, scoped to a single user.
type FileSystem = webdav.FileSystem

type fsImpl struct {
	svc    *storage.FileService
	userID string
}

// NewFileSystem constructs a webdav.FileSystem for a specific user's file tree.
func NewFileSystem(svc *storage.FileService, userID string) webdav.FileSystem {
	return &fsImpl{svc: svc, userID: userID}
}

func clean(name string) string { return path.Clean("/" + strings.TrimPrefix(name, "/")) }

// isMacJunk detects macOS Finder housekeeping files created during WebDAV copies:
// AppleDouble sidecar files (._name, resource fork/xattrs) and .DS_Store (folder view metadata).
// We do not materialize them — writes are accepted and discarded to keep
// the file tree and web UI clean. The actual file data is stored as normal.
func isMacJunk(name string) bool {
	return name == ".DS_Store" || strings.HasPrefix(name, "._")
}

// deviceOf reads the deviceID from ctx (set by the middleware); falls back to "webdav" in tests.
func deviceOf(ctx context.Context) string {
	if d, ok := ctx.Value(ctxDeviceKey).(string); ok && d != "" {
		return d
	}
	return "webdav"
}

func (f *fsImpl) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	name = clean(name)
	if name == "/" {
		return nodeInfo{name: "/", dir: true}, nil
	}
	if isMacJunk(path.Base(name)) {
		return nil, os.ErrNotExist // macOS junk files are not exposed
	}
	n, err := f.svc.NodeByPath(ctx, f.userID, name)
	if err != nil {
		return nil, mapErr(err)
	}
	return infoFromNode(n), nil
}

func (f *fsImpl) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	name = clean(name)
	parentID, leaf, err := f.parentOf(ctx, name)
	if err != nil {
		return err
	}
	if isMacJunk(leaf) {
		return nil // pretend it was created — nothing is materialized
	}
	_, err = f.svc.CreateFolder(ctx, f.userID, parentID, leaf)
	return mapErr(err)
}

func (f *fsImpl) RemoveAll(ctx context.Context, name string) error {
	name = clean(name)
	if isMacJunk(path.Base(name)) {
		return nil // macOS junk files do not exist — nothing to delete
	}
	n, err := f.svc.NodeByPath(ctx, f.userID, name)
	if err != nil {
		return mapErr(err)
	}
	return mapErr(f.svc.Delete(ctx, f.userID, db.UUIDString(n.ID)))
}

// Rename implements WebDAV MOVE. NOTE: changing both directory and name simultaneously
// requires two core operations (Move then Rename) — not atomic.
// A pure move and a pure rename are each atomic; doing both at once is a known
// limitation of step 1.1 (atomic MoveAndRename in the core is a separate step).
func (f *fsImpl) Rename(ctx context.Context, oldName, newName string) error {
	oldName, newName = clean(oldName), clean(newName)
	if isMacJunk(path.Base(oldName)) || isMacJunk(path.Base(newName)) {
		return nil // macOS junk files are not materialized — nothing to move
	}
	n, err := f.svc.NodeByPath(ctx, f.userID, oldName)
	if err != nil {
		return mapErr(err)
	}
	id := db.UUIDString(n.ID)
	if path.Dir(oldName) != path.Dir(newName) {
		newParentID, _, err := f.parentOf(ctx, newName)
		if err != nil {
			return err
		}
		if _, err := f.svc.Move(ctx, f.userID, id, newParentID); err != nil {
			return mapErr(err)
		}
	}
	if path.Base(oldName) != path.Base(newName) {
		if _, err := f.svc.Rename(ctx, f.userID, id, path.Base(newName)); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func (f *fsImpl) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	name = clean(name)
	leaf := path.Base(name)
	if flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0 {
		if isMacJunk(leaf) {
			return &discardFile{name: leaf}, nil // accept and discard
		}
		parentID, leaf, err := f.parentOf(ctx, name)
		if err != nil {
			return nil, err
		}
		tmp, err := os.CreateTemp("", "kfdav-*")
		if err != nil {
			return nil, err
		}
		return &writeFile{ctx: ctx, svc: f.svc, userID: f.userID, deviceID: deviceOf(ctx),
			parentID: parentID, name: leaf, tmp: tmp}, nil
	}
	if name == "/" {
		return f.dir(ctx, nodeInfo{name: "/", dir: true}, "")
	}
	if isMacJunk(leaf) {
		return nil, os.ErrNotExist // macOS junk files do not exist for reading
	}
	n, err := f.svc.NodeByPath(ctx, f.userID, name)
	if err != nil {
		return nil, mapErr(err)
	}
	if n.IsDir {
		return f.dir(ctx, infoFromNode(n), db.UUIDString(n.ID))
	}
	// Lazy: defer opening the backing content until the first Read/Seek so a metadata-only
	// PROPFIND never touches file bytes (see readFile).
	id := db.UUIDString(n.ID)
	return &readFile{
		info: infoFromNode(n),
		open: func() (*os.File, error) {
			_, file, err := f.svc.Open(ctx, f.userID, id)
			if err != nil {
				return nil, mapErr(err)
			}
			return file, nil
		},
	}, nil
}

// dir builds a directory webdav.File: lists children (root or a named node).
func (f *fsImpl) dir(ctx context.Context, info nodeInfo, nodeID string) (webdav.File, error) {
	var (
		kids []db.Node
		err  error
	)
	if nodeID == "" {
		kids, err = f.svc.RootChildren(ctx, f.userID)
	} else {
		kids, err = f.svc.ListChildren(ctx, f.userID, nodeID)
	}
	if err != nil {
		return nil, mapErr(err)
	}
	children := make([]fs.FileInfo, 0, len(kids))
	for _, k := range kids {
		// Don't expose macOS junk nodes (.DS_Store, ._*) over WebDAV. Besides being noise,
		// Stat()/OpenFile() report them as non-existent, which would abort the whole PROPFIND
		// walk if they slipped into a listing.
		if isMacJunk(k.Name) {
			continue
		}
		children = append(children, infoFromNode(k))
	}
	return &dirFile{info: info, children: children}, nil
}

// parentOf resolves the parent directory of a path → its nodeID (nil = root) and the leaf name.
func (f *fsImpl) parentOf(ctx context.Context, name string) (*string, string, error) {
	dir, leaf := path.Dir(name), path.Base(name)
	if dir == "/" || dir == "." {
		return nil, leaf, nil
	}
	pn, err := f.svc.NodeByPath(ctx, f.userID, dir)
	if err != nil {
		return nil, "", mapErr(err)
	}
	id := db.UUIDString(pn.ID)
	return &id, leaf, nil
}
