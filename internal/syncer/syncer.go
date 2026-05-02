// Package syncer polls the Journal server's price-snapshot endpoint on a
// schedule and writes JournalPrices.lua to the ESO SavedVariables directory.
//
// The Syncer runs as an independent goroutine. It is started by the tray's
// onReady alongside the Watcher and shares the same context for shutdown.
// Errors are logged and never crash the caller — the next tick retries.
package syncer

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// StatusReporter receives sync-status updates after each sync attempt.
// The tray implements this interface so the Syncer doesn't depend on the tray
// package directly.
type StatusReporter interface {
	UpdatePriceSyncStatus(SyncStatus)
}

// SyncStatus is a snapshot of the Syncer's state, delivered to the tray
// after each sync attempt.
type SyncStatus struct {
	LastSyncAt time.Time
	LastError  error
	Enabled    bool
}

// Config carries the runtime parameters for a Syncer. All fields are immutable
// after construction; the enabled flag is toggled via SetEnabled.
type Config struct {
	Interval    time.Duration
	SnapshotURL string
	AddonDir    string // path to Journal addon folder; JournalPrices.lua is written here
	// TokenFunc returns the current device Bearer token. Called on each request.
	// Returns an empty string when the user is not signed in.
	TokenFunc func() string
}

// Syncer polls the snapshot endpoint on a schedule and writes the result to
// the ESO SavedVariables directory.
type Syncer struct {
	cfg        Config
	httpClient *http.Client
	reporter   StatusReporter

	enabled atomic.Bool

	mu           sync.Mutex
	lastETag     string
	lastModified string
}

// New creates a Syncer. Call Run to start the poll loop.
func New(cfg Config, enabled bool, reporter StatusReporter) *Syncer {
	s := &Syncer{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		reporter:   reporter,
	}
	s.enabled.Store(enabled)
	return s
}

// SetEnabled toggles the Syncer. Takes effect on the next tick.
func (s *Syncer) SetEnabled(v bool) {
	s.enabled.Store(v)
}

// IsEnabled reports the current toggle state.
func (s *Syncer) IsEnabled() bool {
	return s.enabled.Load()
}

// TriggerSync fires an immediate sync in a new goroutine, independent of the
// regular poll interval. Used when the user enables syncing via the tray toggle
// so they don't have to wait up to an hour for the first sync.
func (s *Syncer) TriggerSync(ctx context.Context) {
	go s.syncOnce(ctx)
}

// Run performs an immediate sync (if enabled) and then polls on cfg.Interval
// until ctx is cancelled.
func (s *Syncer) Run(ctx context.Context) {
	if s.enabled.Load() {
		s.syncOnce(ctx)
	}

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.enabled.Load() {
				continue
			}
			s.syncOnce(ctx)
		}
	}
}

func (s *Syncer) syncOnce(ctx context.Context) {
	err := s.doSync(ctx)
	if err != nil {
		slog.Error("price sync failed", "err", err)
	}
	s.reporter.UpdatePriceSyncStatus(SyncStatus{
		LastSyncAt: time.Now(),
		LastError:  err,
		Enabled:    s.enabled.Load(),
	})
}
