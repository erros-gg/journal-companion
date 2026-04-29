// Package watcher implements fsnotify-based file watching with debounce and
// hash-based deduplication (Phase 3).
//
// Phase 3 design:
//   - Watch the specific file paths listed in config (not entire directories).
//   - On WRITE/CREATE event debounce rapid bursts within the configured window.
//   - Hash the file after the debounce delay; skip upload if hash unchanged.
//   - Handle file locks with read-retry and backoff (ESO holds the file briefly).
//   - The watcher is restarted whenever the config changes (new paths, new debounce).
package watcher

// TODO Phase 3: implement Watch(ctx, cfg, db, uploadFn) that:
//   - starts an fsnotify watcher on each cfg.Watch path
//   - debounces WRITE events using cfg.Behavior.DebounceMs
//   - compares new hash against db.WatchHash; skips if identical
//   - calls uploadFn(path, label) on genuine changes
//   - calls db.SetWatchHash on successful upload
//   - stops cleanly when ctx is cancelled
