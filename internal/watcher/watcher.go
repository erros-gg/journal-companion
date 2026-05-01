// Package watcher implements fsnotify-based file watching with debounce and
// hash-based deduplication. It is the Phase 3 core: automatic uploads on change.
package watcher

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/erros-gg/journal-companion/internal/config"
	"github.com/erros-gg/journal-companion/internal/db"
	"github.com/erros-gg/journal-companion/internal/platform"
	"github.com/fsnotify/fsnotify"
)

// UploadFunc is invoked when a watched file has genuinely changed.
// Returns the uploadID assigned by the server (non-empty on success).
// A non-nil error prevents the stored hash from being updated, so the next
// change attempt will retry. Returning ("", nil) is the signed-out no-op case
// and also skips the hash update.
type UploadFunc func(label, path string) (uploadID string, err error)

// Watcher watches configured file paths and triggers uploads on genuine changes.
type Watcher struct {
	cfg         *config.Config
	db          *db.DB
	uploadFn    UploadFunc
	paused      atomic.Int32 // 1 = paused, 0 = active
	pathToLabel map[string]string
}

// New creates a Watcher. Call Start to begin watching.
func New(cfg *config.Config, database *db.DB, uploadFn UploadFunc) *Watcher {
	return &Watcher{
		cfg:         cfg,
		db:          database,
		uploadFn:    uploadFn,
		pathToLabel: make(map[string]string),
	}
}

// Pause stops event processing. Events received while paused are dropped.
func (w *Watcher) Pause() { w.paused.Store(1) }

// Resume restarts event processing. Does not retroactively process missed events.
func (w *Watcher) Resume() { w.paused.Store(0) }

// IsPaused returns true if the watcher is currently paused.
func (w *Watcher) IsPaused() bool { return w.paused.Load() == 1 }

// Start begins watching all configured paths and blocks until ctx is cancelled.
// If no paths are configured, it logs and idles until cancellation.
// Errors inside the watch loop are logged and never crash the caller.
func (w *Watcher) Start(ctx context.Context) error {
	if len(w.cfg.Watch) == 0 {
		slog.Info("watcher: no watch paths configured, idling")
		<-ctx.Done()
		return ctx.Err()
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	defer fw.Close()

	for _, wc := range w.cfg.Watch {
		if err := fw.Add(wc.Path); err != nil {
			// Not fatal — file may not exist yet or path may be misconfigured.
			slog.Warn("watcher: could not watch path", "path", wc.Path, "err", err)
			continue
		}
		w.pathToLabel[wc.Path] = wc.Label
		slog.Info("watcher: watching", "path", wc.Path, "label", wc.Label)
	}

	debounceDur := time.Duration(w.cfg.Behavior.DebounceMs) * time.Millisecond
	if debounceDur == 0 {
		debounceDur = 500 * time.Millisecond
	}

	db := &debounce{
		timers: make(map[string]*time.Timer),
		delay:  debounceDur,
		fn:     w.processChange,
	}

	for {
		select {
		case <-ctx.Done():
			db.stopAll()
			return ctx.Err()

		case event, ok := <-fw.Events:
			if !ok {
				return errors.New("fsnotify events channel closed")
			}
			if !event.Has(fsnotify.Write) {
				continue
			}
			if w.paused.Load() == 1 {
				continue // drop while paused — do not queue
			}
			if _, watched := w.pathToLabel[event.Name]; !watched {
				continue
			}
			db.trigger(event.Name)

		case err, ok := <-fw.Errors:
			if !ok {
				return errors.New("fsnotify errors channel closed")
			}
			slog.Error("watcher: fsnotify error", "err", err)
			// Continue — transient errors must not crash the daemon.
		}
	}
}

// processChange is called by the debouncer after the quiet window expires.
// It hashes the file, checks for genuine change, and triggers an upload.
// All errors are logged; none are returned (this runs in a timer goroutine).
func (w *Watcher) processChange(path string) {
	label := w.pathToLabel[path]

	data, err := readWithRetry(path)
	if err != nil {
		slog.Error("watcher: read file failed after retries", "path", path, "err", err)
		return
	}

	sum := sha256.Sum256(data)
	hash := fmt.Sprintf("sha256:%x", sum[:])

	storedHash, err := w.db.WatchHash(path)
	if err != nil {
		slog.Error("watcher: lookup stored hash", "path", path, "err", err)
		return
	}
	if hash == storedHash {
		slog.Info("watcher: file unchanged (hash match), skipping upload", "path", path)
		return
	}

	slog.Info("watcher: detected change, uploading", "path", path, "label", label)
	uploadID, err := w.uploadFn(label, path)
	if err != nil {
		slog.Error("watcher: upload failed, hash not updated", "path", path, "err", err)
		// Do NOT update hash — ensures next change retries the upload.
		return
	}
	if uploadID == "" {
		// No-op (e.g. not signed in). Do not update hash so it retries after sign-in.
		return
	}

	if err := w.db.SetWatchHash(path, hash, uploadID); err != nil {
		slog.Error("watcher: store hash", "path", path, "err", err)
	}
}

// readWithRetry reads the file at path, retrying with backoff if the file is
// locked by another process (common on Windows when ESO is still flushing).
// Retry schedule: immediate, then 100 ms, 500 ms, 2 s — 4 attempts total.
func readWithRetry(path string) ([]byte, error) {
	delays := []time.Duration{0, 100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}
	var lastErr error
	for _, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !platform.IsFileLocked(err) {
			return nil, err // non-lock error, no point retrying
		}
		lastErr = err
		slog.Warn("watcher: file locked, will retry", "path", path, "backoff", delay)
	}
	return nil, fmt.Errorf("file locked after retries: %w", lastErr)
}

// debounce coalesces rapid events per path into a single callback invocation.
type debounce struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
	delay  time.Duration
	fn     func(string)
}

func (d *debounce) trigger(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[path]; ok {
		t.Reset(d.delay)
	} else {
		d.timers[path] = time.AfterFunc(d.delay, func() {
			d.mu.Lock()
			delete(d.timers, path)
			d.mu.Unlock()
			d.fn(path)
		})
	}
}

func (d *debounce) stopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for path, t := range d.timers {
		t.Stop()
		delete(d.timers, path)
	}
}
