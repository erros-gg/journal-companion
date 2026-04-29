// Package watcher implements fsnotify-based file watching (Phase 3).
package watcher

import "errors"

// ErrNotImplemented is returned by all watcher functions until Phase 3.
var ErrNotImplemented = errors.New("not yet implemented")

// TODO Phase 3: implement Watch(ctx, cfg, db, uploadFn) that:
//   - starts an fsnotify watcher on each cfg.Watch path
//   - debounces WRITE events using cfg.Behavior.DebounceMs
//   - compares new hash against db.WatchHash; skips if identical
//   - calls uploadFn(path, label) on genuine changes
//   - calls db.SetWatchHash on successful upload
//   - stops cleanly when ctx is cancelled
