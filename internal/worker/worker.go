// Package worker runs discodrive background jobs without Redis: periodic
// tickers for version/trash GC and rescan, plus an fsnotify watcher that triggers
// a rescan almost instantly when files change on disk outside the service.
package worker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"discodrive/internal/db"
	"discodrive/internal/ebook"
	"discodrive/internal/music"
	"discodrive/internal/notify"
	"discodrive/internal/podcast"
	"discodrive/internal/storage"
)

// Config holds intervals and parameters for background jobs.
type Config struct {
	RescanInterval    time.Duration
	TrimInterval      time.Duration
	TrashInterval     time.Duration
	TrashRetention    time.Duration
	QuotaInterval     time.Duration
	PairingGCInterval time.Duration
	VersionKeep       int
	Debounce          time.Duration

	PodcastRefreshInterval time.Duration
	PodcastKeepPerChannel  int
}

// Default returns sensible defaults; keep/trashDays/rescanSeconds come from config.
func Default(versionKeep, trashDays, rescanSeconds int) Config {
	return Config{
		RescanInterval:    time.Duration(rescanSeconds) * time.Second,
		TrimInterval:      60 * time.Second,
		TrashInterval:     time.Hour,
		TrashRetention:    time.Duration(trashDays) * 24 * time.Hour,
		QuotaInterval:     15 * time.Minute,
		PairingGCInterval: 5 * time.Minute,
		VersionKeep:       versionKeep,
		Debounce:          500 * time.Millisecond,

		PodcastRefreshInterval: 6 * time.Hour,
		PodcastKeepPerChannel:  5,
	}
}

type Worker struct {
	fs       *storage.FileService
	root     string
	q        *db.Queries
	notify   *notify.Notifier
	cfg      Config
	idx      *music.Indexer // nil if music indexing is not configured
	ebookIdx *ebook.Indexer // nil if ebook indexing is not configured
}

func New(fs *storage.FileService, root string, q *db.Queries, notifier *notify.Notifier, cfg Config, idx *music.Indexer, ebookIdx *ebook.Indexer) *Worker {
	return &Worker{fs: fs, root: root, q: q, notify: notifier, cfg: cfg, idx: idx, ebookIdx: ebookIdx}
}

// Run starts all background jobs and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	go w.tick(ctx, w.cfg.RescanInterval, "rescan", func(ctx context.Context) error {
		return w.fs.Rescan(ctx)
	})
	go w.tick(ctx, w.cfg.TrimInterval, "trim-versions", func(ctx context.Context) error {
		return w.fs.TrimVersions(ctx, w.cfg.VersionKeep)
	})
	go w.tick(ctx, w.cfg.TrashInterval, "trash-gc", func(ctx context.Context) error {
		return w.fs.TrashGC(ctx, w.cfg.TrashRetention)
	})
	go w.tick(ctx, w.cfg.QuotaInterval, "quota-notify", w.quotaNotify)
	go w.tick(ctx, w.cfg.PairingGCInterval, "pairing-gc", func(ctx context.Context) error {
		return w.q.DeleteExpiredPairings(ctx)
	})
	go w.tick(ctx, w.cfg.PodcastRefreshInterval, "podcast-refresh", w.podcastRefresh)
	if w.idx != nil {
		go w.tick(ctx, w.cfg.RescanInterval, "music-scan", w.musicScan)
	}
	if w.ebookIdx != nil {
		go w.tick(ctx, w.cfg.RescanInterval, "ebook-scan", w.ebookScan)
	}
	w.watch(ctx)
}

// musicScan indexes audio files for all users who have music enabled.
func (w *Worker) musicScan(ctx context.Context) error {
	users, err := w.q.EnabledMusicUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if !u.FolderNodeID.Valid {
			continue
		}
		userID := db.UUIDString(u.UserID)
		folderID := db.UUIDString(u.FolderNodeID)
		n, err := w.idx.ScanFolder(ctx, userID, folderID)
		if err != nil {
			log.Printf("discodrive: music-scan user=%s: %v", userID, err)
			continue
		}
		if n > 0 {
			log.Printf("discodrive: music-scan user=%s: indexed %d file(s)", userID, n)
		}
	}
	return nil
}

// ebookScan indexes e-book files for all users who have ebook indexing enabled.
func (w *Worker) ebookScan(ctx context.Context) error {
	users, err := w.q.EnabledEbookUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if !u.FolderNodeID.Valid {
			continue
		}
		userID := db.UUIDString(u.UserID)
		folderID := db.UUIDString(u.FolderNodeID)
		n, err := w.ebookIdx.ScanFolder(ctx, userID, folderID)
		if err != nil {
			log.Printf("discodrive: ebook-scan user=%s: %v", userID, err)
			continue
		}
		if n > 0 {
			log.Printf("discodrive: ebook-scan user=%s: indexed %d file(s)", userID, n)
		}
	}
	return nil
}

// podcastRefresh re-fetches every subscribed podcast feed, upserts new episodes,
// refreshes channel metadata, and prunes downloaded episodes beyond the keep limit.
func (w *Worker) podcastRefresh(ctx context.Context) error {
	channels, err := w.q.ListAllPodcastChannels(ctx)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if err := podcast.RefreshChannel(ctx, w.q, w.root, ch); err != nil {
			log.Printf("discodrive: podcast-refresh channel=%s: %v", db.UUIDString(ch.ID), err)
			continue
		}

		// Prune downloaded episodes beyond the keep limit (rows are newest-first).
		rows, _ := w.q.ListCompletedEpisodesByChannelDesc(ctx, ch.ID)
		start := pruneStart(len(rows), w.cfg.PodcastKeepPerChannel)
		for _, row := range rows[start:] {
			if row.DiskPath.Valid {
				_ = os.Remove(filepath.Join(w.root, row.DiskPath.String))
			}
			if err := w.q.ClearEpisodeDownload(ctx, db.ClearEpisodeDownloadParams{ID: row.ID, UserID: ch.UserID}); err != nil {
				log.Printf("discodrive: podcast-refresh prune channel=%s: %v", db.UUIDString(ch.ID), err)
			}
		}
	}
	return nil
}

// pruneStart returns the start index of the prune tail for completed episodes
// ordered newest-first: rows[start:] should be pruned, rows[:start] kept.
func pruneStart(total, keep int) int {
	if keep < 0 {
		keep = 0
	}
	if total <= keep {
		return total
	}
	return keep
}

func (w *Worker) tick(ctx context.Context, every time.Duration, name string, job func(context.Context) error) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := job(ctx); err != nil {
				log.Printf("discodrive: job %s: %v", name, err)
			}
		}
	}
}

// watch monitors the data tree and debounces rescan on disk changes.
func (w *Worker) watch(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("discodrive: fsnotify unavailable: %v", err)
		<-ctx.Done()
		return
	}
	defer watcher.Close()

	if err := os.MkdirAll(w.root, 0o755); err != nil {
		log.Printf("discodrive: creating data root: %v", err)
	}
	w.addRecursive(watcher, w.root)

	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-watcher.Events:
			if !ok {
				return
			}
			// also watch newly created directories
			if e.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(e.Name); err == nil && fi.IsDir() {
					w.addRecursive(watcher, e.Name)
				}
			}
			debounce.Reset(w.cfg.Debounce)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("discodrive: fsnotify: %v", err)
		case <-debounce.C:
			if err := w.fs.Rescan(ctx); err != nil {
				log.Printf("discodrive: rescan (fsnotify): %v", err)
			}
			if w.idx != nil {
				if err := w.musicScan(ctx); err != nil {
					log.Printf("discodrive: music-scan (fsnotify): %v", err)
				}
			}
			if w.ebookIdx != nil {
				if err := w.ebookScan(ctx); err != nil {
					log.Printf("discodrive: ebook-scan (fsnotify): %v", err)
				}
			}
		}
	}
}

// quotaNotify sends a notification to users who have crossed 90% of their quota (once),
// and clears the flag for those who have dropped back below the threshold.
func (w *Worker) quotaNotify(ctx context.Context) error {
	rows, err := w.q.ListQuotaCandidates(ctx)
	if err != nil {
		return err
	}
	for _, u := range rows {
		quota := u.StorageQuota.Int64
		percent := 0
		if quota > 0 {
			percent = int(u.StorageUsed * 100 / quota)
		}
		w.notify.Emit(ctx, db.UUIDString(u.ID), "quota.near_limit", map[string]any{
			"Percent": percent, "Used": u.StorageUsed, "Quota": quota,
		})
		if err := w.q.MarkQuotaNotified(ctx, u.ID); err != nil {
			return err
		}
	}
	return w.q.ClearQuotaNotified(ctx)
}

// addRecursive adds a directory and all its subdirectories to the watcher, skipping
// the internal .versions/.tmp directories (they are outside the tree mirror).
func (w *Worker) addRecursive(watcher *fsnotify.Watcher, dir string) {
	_ = filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil || !e.IsDir() {
			return nil
		}
		if base := filepath.Base(p); strings.HasPrefix(base, ".versions") || strings.HasPrefix(base, ".tmp") {
			return filepath.SkipDir
		}
		_ = watcher.Add(p)
		return nil
	})
}
