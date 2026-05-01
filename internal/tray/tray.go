// Package tray owns the system tray icon and menu — the app's complete UI surface.
package tray

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/erros-gg/journal-companion/internal/auth"
	"github.com/erros-gg/journal-companion/internal/config"
	"github.com/erros-gg/journal-companion/internal/db"
	"github.com/erros-gg/journal-companion/internal/platform"
	"github.com/erros-gg/journal-companion/internal/syncer"
	"github.com/erros-gg/journal-companion/internal/uploader"
	"github.com/erros-gg/journal-companion/internal/watcher"
	"github.com/getlantern/systray"
)

// App carries the dependencies the tray needs to respond to menu events.
type App struct {
	Version    string
	Config     *config.Config
	ConfigPath string
	DB         *db.DB
	AppDir     string

	cancelWatcher context.CancelFunc
	syncCtx       context.Context
	priceSyncer   *syncer.Syncer // nil if no eso-savedvariables watch is configured
}

// trayState holds live menu-item references and mutable auth/watcher state.
type trayState struct {
	mu sync.Mutex

	mAuthStatus   *systray.MenuItem
	mUploadStatus *systray.MenuItem
	mPriceStatus  *systray.MenuItem
	mUploadNow    *systray.MenuItem
	mPause        *systray.MenuItem
	mSyncPrices   *systray.MenuItem
	mSignIn       *systray.MenuItem
	mSignOut      *systray.MenuItem

	token    string
	username string
	paused   bool

	iconMu      sync.Mutex
	iconWorking bool
	iconStarted time.Time
}

var state trayState

// Run starts the system tray and blocks until the user chooses Quit.
// Must be called on the main goroutine (systray requirement).
func Run(app *App) {
	systray.Run(app.onReady, app.onExit)
}

func (a *App) onReady() {
	systray.SetIcon(iconDefault())
	systray.SetTooltip("Journal Companion — not signed in")

	state.mAuthStatus = systray.AddMenuItem("Not signed in", "")
	state.mAuthStatus.Disable()
	state.mUploadStatus = systray.AddMenuItem("Last upload: never", "")
	state.mUploadStatus.Disable()
	state.mPriceStatus = systray.AddMenuItem("Prices: starting…", "")
	state.mPriceStatus.Disable()

	systray.AddSeparator()

	state.mPause = systray.AddMenuItem("Pause watching", "Pause automatic uploads")
	state.mUploadNow = systray.AddMenuItem("Upload now", "Upload the watched file immediately")
	if len(a.Config.Watch) == 0 {
		state.mPause.Disable()
	}
	state.mUploadNow.Disable()
	state.mSyncPrices = systray.AddMenuItem("Sync prices", "Download price data from Journal")

	systray.AddSeparator()

	mActivity := systray.AddMenuItem("View recent activity", "See recent upload history")
	mOpenConfig := systray.AddMenuItem("Open config folder", "Open the config folder")

	systray.AddSeparator()

	mStartWithOS := systray.AddMenuItem("Start with Windows", "Launch automatically at login")
	if a.Config.Behavior.StartWithOS {
		mStartWithOS.Check()
	}

	systray.AddSeparator()

	state.mSignIn = systray.AddMenuItem("Sign in to Journal", "Authenticate with your Journal account")
	state.mSignOut = systray.AddMenuItem("Sign out", "Remove stored credentials")
	state.mSignOut.Hide()
	mAbout := systray.AddMenuItem("About Journal Companion", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Journal Companion")

	a.loadStoredCredentials()

	watchCtx, cancelWatcher := context.WithCancel(context.Background())
	a.cancelWatcher = cancelWatcher
	a.syncCtx = watchCtx
	w := watcher.New(a.Config, a.DB, a.makeUploadCallback())
	a.restoreWatcherState(w)
	go a.runWatcher(watchCtx, w)

	a.startSyncer(watchCtx)

	go a.handleEvents(w, mActivity, mOpenConfig, mStartWithOS, mAbout, mQuit)
}

func (a *App) restoreWatcherState(w *watcher.Watcher) {
	if v, _ := a.DB.GetSetting("watcher_paused"); v == "1" {
		w.Pause()
		state.mu.Lock()
		state.paused = true
		state.mu.Unlock()
		state.mPause.SetTitle("Resume watching")
	}
}

func (a *App) runWatcher(ctx context.Context, w *watcher.Watcher) {
	if err := w.Start(ctx); err != nil && ctx.Err() == nil {
		slog.Error("watcher exited unexpectedly", "err", err)
	}
}

func (a *App) makeUploadCallback() watcher.UploadFunc {
	return func(label, path string) (string, error) {
		state.mu.Lock()
		token := state.token
		state.mu.Unlock()

		if token == "" {
			slog.Info("watcher: skipping upload — not signed in", "path", path)
			return "", nil
		}

		a.setIconWorking()
		client := uploader.New(a.Config.Network.APIBase, a.Version)
		result, err := client.Upload(token, label, path, a.DB)
		a.setIconDone()

		if err != nil {
			if err.Error() == "token rejected — sign in again" {
				a.setDisconnected()
			}
			return "", err
		}

		ts := time.Now().Format("3:04 PM")
		a.setUploadStatus(fmt.Sprintf("%s — %s", ts, result.Summary))
		return result.UploadID, nil
	}
}

func (a *App) loadStoredCredentials() {
	token, _ := auth.LoadToken()
	if token == "" {
		return
	}
	username, _ := auth.LoadUsername()
	a.setConnected(token, username)
}

func (a *App) setConnected(token, username string) {
	state.mu.Lock()
	state.token = token
	state.username = username
	paused := state.paused
	state.mu.Unlock()

	label := "Connected to Journal"
	if username != "" {
		label = "Connected as @" + username
	}
	state.mAuthStatus.SetTitle(label)

	if paused {
		systray.SetTooltip("Paused — not watching for changes")
	} else {
		systray.SetTooltip(label)
	}

	state.mSignIn.Hide()
	state.mSignOut.Show()

	if len(a.Config.Watch) > 0 {
		state.mUploadNow.Enable()
	}
}

func (a *App) setDisconnected() {
	state.mu.Lock()
	state.token = ""
	state.username = ""
	state.mu.Unlock()

	state.mAuthStatus.SetTitle("Not signed in")
	systray.SetTooltip("Journal Companion — not signed in")
	state.mSignOut.Hide()
	state.mSignIn.Show()
	state.mUploadNow.Disable()
}

func (a *App) setUploadStatus(summary string) {
	state.mUploadStatus.SetTitle("Last upload: " + summary)
}

func (a *App) setIconWorking() {
	state.iconMu.Lock()
	state.iconWorking = true
	state.iconStarted = time.Now()
	state.iconMu.Unlock()
	systray.SetIcon(iconWorking())
}

func (a *App) setIconDone() {
	const minDisplay = 200 * time.Millisecond
	state.iconMu.Lock()
	elapsed := time.Since(state.iconStarted)
	state.iconWorking = false
	state.iconMu.Unlock()

	if elapsed < minDisplay {
		time.Sleep(minDisplay - elapsed)
	}
	systray.SetIcon(iconDefault())
}

func (a *App) handleEvents(
	w *watcher.Watcher,
	mActivity, mOpenConfig, mStartWithOS, mAbout, mQuit *systray.MenuItem,
) {
	for {
		select {
		case <-state.mPause.ClickedCh:
			a.togglePause(w)

		case <-state.mSyncPrices.ClickedCh:
			a.toggleSyncPrices()

		case <-mOpenConfig.ClickedCh:
			if err := platform.OpenFolder(a.AppDir); err != nil {
				slog.Error("open config folder", "err", err)
			}

		case <-state.mSignIn.ClickedCh:
			go a.startAuthFlow()

		case <-state.mSignOut.ClickedCh:
			a.signOut()

		case <-state.mUploadNow.ClickedCh:
			go a.uploadNow()

		case <-mStartWithOS.ClickedCh:
			slog.Info("TODO: implement Start with OS (Phase 4)")

		case <-mActivity.ClickedCh:
			go a.showRecentActivity()

		case <-mAbout.ClickedCh:
			slog.Info("about", "version", a.Version)

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) togglePause(w *watcher.Watcher) {
	if w.IsPaused() {
		w.Resume()
		state.mu.Lock()
		state.paused = false
		username := state.username
		state.mu.Unlock()

		state.mPause.SetTitle("Pause watching")
		_ = a.DB.SetSetting("watcher_paused", "0")

		if username != "" {
			systray.SetTooltip("Connected as @" + username)
		} else {
			systray.SetTooltip("Journal Companion — not signed in")
		}
		slog.Info("watcher: resumed")
	} else {
		w.Pause()
		state.mu.Lock()
		state.paused = true
		state.mu.Unlock()

		state.mPause.SetTitle("Resume watching")
		_ = a.DB.SetSetting("watcher_paused", "1")
		systray.SetTooltip("Paused — not watching for changes")
		slog.Info("watcher: paused")
	}
}

func (a *App) startAuthFlow() {
	state.mSignIn.Disable()
	state.mAuthStatus.SetTitle("Signing in…")
	defer state.mSignIn.Enable()

	result, err := auth.StartFlow(a.Config.Network.APIBase, "Companion App on Windows")
	if err != nil {
		slog.Error("auth flow failed", "err", err)
		state.mAuthStatus.SetTitle("Sign in failed — try again")
		return
	}
	a.setConnected(result.Token, result.Username)
}

func (a *App) signOut() {
	if err := auth.DeleteCredentials(); err != nil {
		slog.Error("delete credentials", "err", err)
	}
	a.setDisconnected()
	slog.Info("signed out")
}

func (a *App) uploadNow() {
	state.mu.Lock()
	token := state.token
	state.mu.Unlock()

	if token == "" {
		slog.Warn("upload now: not signed in")
		return
	}
	if len(a.Config.Watch) == 0 {
		slog.Warn("upload now: no watch paths configured")
		return
	}

	state.mUploadNow.Disable()
	a.setUploadStatus("uploading…")
	defer state.mUploadNow.Enable()

	a.setIconWorking()
	client := uploader.New(a.Config.Network.APIBase, a.Version)

	var lastSummary string
	for _, wc := range a.Config.Watch {
		result, err := client.Upload(token, wc.Label, wc.Path, a.DB)
		if err != nil {
			slog.Error("upload", "label", wc.Label, "path", wc.Path, "err", err)
			if result.Status == "network_error" {
				a.setUploadStatus("Upload pending — check connection")
			}
			if err.Error() == "token rejected — sign in again" {
				a.setIconDone()
				a.setDisconnected()
				return
			}
			continue
		}
		lastSummary = result.Summary
	}
	a.setIconDone()

	if lastSummary != "" {
		ts := time.Now().Format("3:04 PM")
		a.setUploadStatus(fmt.Sprintf("%s — %s", ts, lastSummary))
	}
}

func (a *App) showRecentActivity() {
	uploads, err := a.DB.RecentUploads(10)
	if err != nil {
		slog.Error("recent uploads", "err", err)
		return
	}
	if len(uploads) == 0 {
		slog.Info("no upload history")
		return
	}
	for _, u := range uploads {
		slog.Info("upload history",
			"id", u.ID,
			"label", u.Label,
			"status", u.Status,
			"queued", u.QueuedAt.Format(time.RFC3339),
		)
	}
}

func (a *App) onExit() {
	if a.cancelWatcher != nil {
		a.cancelWatcher()
	}
	slog.Info("exiting cleanly")
}

// ── Price sync ───────────────────────────────────────────────────────────────

func (a *App) startSyncer(ctx context.Context) {
	addonDir, err := syncer.DeriveAddonDir(a.Config.Watch)
	if err != nil {
		slog.Info("price sync unavailable", "reason", err)
		state.mPriceStatus.SetTitle("Prices: not configured")
		state.mSyncPrices.Hide()
		return
	}

	interval := time.Duration(a.Config.Behavior.SyncPricesIntervalMin) * time.Minute
	if interval == 0 {
		interval = 60 * time.Minute
	}

	snapshotURL := a.Config.Network.SnapshotURL
	if snapshotURL == "" {
		snapshotURL = a.Config.Network.APIBase + "/api/snapshots/eso-prices/latest"
	}

	cfg := syncer.Config{
		Interval:    interval,
		SnapshotURL: snapshotURL,
		AddonDir:    addonDir,
		TokenFunc: func() string {
			state.mu.Lock()
			defer state.mu.Unlock()
			return state.token
		},
	}
	slog.Info("price syncer configured", "url", snapshotURL, "dir", addonDir)
	s := syncer.New(cfg, a.Config.Behavior.SyncPrices, a)
	a.priceSyncer = s

	if a.Config.Behavior.SyncPrices {
		state.mSyncPrices.Check()
		state.mPriceStatus.SetTitle("Prices: never synced")
	} else {
		state.mPriceStatus.SetTitle("Prices: disabled")
	}

	go s.Run(ctx)
}

// UpdatePriceSyncStatus implements syncer.StatusReporter.
// Called from the syncer goroutine after each sync attempt.
func (a *App) UpdatePriceSyncStatus(status syncer.SyncStatus) {
	state.mPriceStatus.SetTitle(priceSyncStatusText(status))
}

func (a *App) toggleSyncPrices() {
	a.Config.Behavior.SyncPrices = !a.Config.Behavior.SyncPrices
	if err := config.Save(a.ConfigPath, a.Config); err != nil {
		slog.Error("save config after sync prices toggle", "err", err)
	}
	if a.Config.Behavior.SyncPrices {
		state.mSyncPrices.Check()
		state.mPriceStatus.SetTitle("Prices: syncing…")
		if a.priceSyncer != nil {
			a.priceSyncer.SetEnabled(true)
			a.priceSyncer.TriggerSync(a.syncCtx)
		}
	} else {
		state.mSyncPrices.Uncheck()
		state.mPriceStatus.SetTitle("Prices: disabled")
		if a.priceSyncer != nil {
			a.priceSyncer.SetEnabled(false)
		}
	}
	slog.Info("price sync toggled", "enabled", a.Config.Behavior.SyncPrices)
}

func priceSyncStatusText(status syncer.SyncStatus) string {
	if !status.Enabled {
		return "Prices: disabled"
	}
	if status.LastSyncAt.IsZero() {
		return "Prices: never synced"
	}
	if status.LastError != nil {
		return "Prices: failed"
	}
	age := time.Since(status.LastSyncAt)
	if age < time.Hour {
		m := int(age.Minutes())
		if m < 1 {
			m = 1
		}
		return fmt.Sprintf("Prices: synced %dm ago", m)
	}
	return fmt.Sprintf("Prices: synced %dh ago", int(age.Hours()))
}
