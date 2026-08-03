package storage_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"discodrive/internal/storage"
)

// blockingReader signals when Read is first called and then blocks until
// released — simulates a client that stalls mid-chunk (network drop, closed
// tab), which keeps the session mutex held inside Chunk→Append.
type blockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingReader() *blockingReader {
	return &blockingReader{started: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return 0, io.EOF
}

// A stalled in-flight chunk must not freeze the whole upload subsystem: GC has
// to finish promptly and Init has to keep working. Regression for the prod
// freeze of 2026-07-22, where GC blocked on a busy session while holding the
// global mutex and every /upload/init returned 504 for 11 hours.
func TestUploads_GCDoesNotBlockBehindInFlightChunk(t *testing.T) {
	u := storage.NewUploads(storage.NewLocalDisk(t.TempDir()), nil)

	id, err := u.Init(context.Background(), "u1", nil, "f.txt", 0, storage.PushMeta{})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	br := newBlockingReader()
	defer close(br.release)
	go func() { _, _ = u.Chunk(context.Background(), id, "u1", 0, br) }()
	<-br.started // Chunk now holds the session mutex, blocked on the client read

	gcDone := make(chan struct{})
	go func() { u.GC(time.Hour); close(gcDone) }()
	select {
	case <-gcDone:
	case <-time.After(2 * time.Second):
		t.Fatal("GC blocked behind an in-flight chunk")
	}

	initDone := make(chan struct{})
	go func() {
		if _, err := u.Init(context.Background(), "u1", nil, "g.txt", 0, storage.PushMeta{}); err != nil {
			t.Errorf("init during in-flight chunk: %v", err)
		}
		close(initDone)
	}()
	select {
	case <-initDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Init blocked while a chunk was in flight (global mutex stuck)")
	}
}

// GC must never reap a session whose chunk is being appended right now, even
// if lastTouch already looks stale (a big chunk over a slow uplink can easily
// outlive the idle cutoff).
func TestUploads_GCSkipsActivelyUploadingSession(t *testing.T) {
	u := storage.NewUploads(storage.NewLocalDisk(t.TempDir()), nil)

	id, err := u.Init(context.Background(), "u1", nil, "f.txt", 0, storage.PushMeta{})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	br := newBlockingReader()
	go func() { _, _ = u.Chunk(context.Background(), id, "u1", 0, br) }()
	<-br.started

	gcDone := make(chan struct{})
	go func() { u.GC(0); close(gcDone) }() // maxAge 0: everything is "idle" by age
	select {
	case <-gcDone:
	case <-time.After(2 * time.Second):
		t.Fatal("GC blocked behind an in-flight chunk")
	}

	close(br.release) // let the chunk finish
	deadline := time.After(2 * time.Second)
	for {
		if next, err := u.Status(id, "u1"); err == nil {
			if next != 1 {
				t.Fatalf("next chunk = %d, want 1", next)
			}
			break
		} else if errors.Is(err, storage.ErrUploadNotFound) {
			t.Fatal("GC reaped a session with an in-flight chunk")
		}
		select {
		case <-deadline:
			t.Fatal("chunk never finished")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
