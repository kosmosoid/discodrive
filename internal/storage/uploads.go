package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"
)

var (
	ErrUploadNotFound  = errors.New("upload session not found")
	ErrChunkOutOfOrder = errors.New("chunk out of order")
)

type uploadSession struct {
	mu        sync.Mutex
	userID    string
	parentID  *string
	name      string
	tmpRel    string
	nextChunk int
	lastTouch time.Time
}

// Uploads manages resumable chunked uploads: sessions are held in memory,
// data is staged in .uploads/<id>, and on completion the assembled file goes through
// Push (versioning/conflicts). Chunking is a transport concern; the file lands on disk whole.
type Uploads struct {
	mu sync.Mutex
	m  map[string]*uploadSession
	st Storage
	fs *FileService
}

func NewUploads(st Storage, fs *FileService) *Uploads {
	return &Uploads{m: make(map[string]*uploadSession), st: st, fs: fs}
}

// Init creates an upload session and returns its ID.
func (u *Uploads) Init(userID string, parentID *string, name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	id := randomHex()
	u.mu.Lock()
	u.m[id] = &uploadSession{userID: userID, parentID: parentID, name: name, tmpRel: ".uploads/" + id, lastTouch: time.Now()}
	u.mu.Unlock()
	return id, nil
}

// GC removes sessions idle for longer than maxAge, deleting their staged temp files.
// Prevents abandoned resumable uploads from leaking memory and disk indefinitely.
func (u *Uploads) GC(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	u.mu.Lock()
	var stale []*uploadSession
	for id, s := range u.m {
		s.mu.Lock()
		idle := s.lastTouch.Before(cutoff)
		s.mu.Unlock()
		if idle {
			stale = append(stale, s)
			delete(u.m, id)
		}
	}
	u.mu.Unlock()
	for _, s := range stale {
		_ = u.st.Remove(s.tmpRel)
	}
}

// StartGC runs GC on a ticker until ctx is cancelled.
func (u *Uploads) StartGC(ctx context.Context, every, maxAge time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u.GC(maxAge)
		}
	}
}

func (u *Uploads) get(id, userID string) (*uploadSession, error) {
	u.mu.Lock()
	s, ok := u.m[id]
	u.mu.Unlock()
	if !ok || s.userID != userID {
		return nil, ErrUploadNotFound
	}
	return s, nil
}

// Chunk appends chunk n. Returns the next expected chunk number.
// An already-accepted chunk is ignored (idempotent); a future one → ErrChunkOutOfOrder.
func (u *Uploads) Chunk(id, userID string, n int, r io.Reader) (int, error) {
	s, err := u.get(id, userID)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastTouch = time.Now()

	switch {
	case n < s.nextChunk:
		_, _ = io.Copy(io.Discard, r)
		return s.nextChunk, nil
	case n > s.nextChunk:
		_, _ = io.Copy(io.Discard, r)
		return s.nextChunk, ErrChunkOutOfOrder
	}
	if err := u.st.Append(s.tmpRel, r); err != nil {
		return s.nextChunk, err
	}
	s.nextChunk++
	return s.nextChunk, nil
}

// Status returns the next expected chunk number (for resuming an upload).
func (u *Uploads) Status(id, userID string) (int, error) {
	s, err := u.get(id, userID)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextChunk, nil
}

// Complete finalizes the upload: the assembled file goes through Push, and the
// session and temp file are removed.
func (u *Uploads) Complete(ctx context.Context, id, userID string) (PushResult, error) {
	s, err := u.get(id, userID)
	if err != nil {
		return PushResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := u.st.Open(s.tmpRel)
	if err != nil {
		return PushResult{}, err
	}
	res, err := u.fs.Push(ctx, s.userID, s.parentID, s.name, nil, "", f)
	_ = f.Close()
	if err != nil {
		return PushResult{}, err
	}
	_ = u.st.Remove(s.tmpRel)
	u.mu.Lock()
	delete(u.m, id)
	u.mu.Unlock()
	return res, nil
}

// Abort cancels an upload: removes the temp file and session. Unknown or foreign ID → no-op.
func (u *Uploads) Abort(userID, id string) {
	u.mu.Lock()
	s, ok := u.m[id]
	if ok && s.userID == userID {
		delete(u.m, id)
	}
	u.mu.Unlock()
	if ok && s.userID == userID {
		_ = u.st.Remove(s.tmpRel)
	}
}

func randomHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
