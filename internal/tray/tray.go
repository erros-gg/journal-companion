// Package tray owns the system tray icon and menu — the app's complete UI surface.
package tray

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/erros-gg/journal-companion/internal/auth"
	"github.com/erros-gg/journal-companion/internal/config"
	"github.com/erros-gg/journal-companion/internal/db"
	"github.com/erros-gg/journal-companion/internal/platform"
	"github.com/erros-gg/journal-companion/internal/uploader"
	"github.com/getlantern/systray"
)

// App carries the dependencies the tray needs to respond to menu events.
type App struct {
	Version    string
	Config     *config.Config
	ConfigPath string
	DB         *db.DB
	AppDir     string
}

// trayState holds live menu-item references and mutable auth state.
// All mutations go through the App setter methods which hold mu.
type trayState struct {
	mu sync.Mutex

	mAuthStatus   *systray.MenuItem
	mUploadStatus *systray.MenuItem
	mUploadNow    *systray.MenuItem
	mSignIn       *systray.MenuItem
	mSignOut      *systray.MenuItem

	token    string
	username string
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

	// ── Status lines (disabled; read-only display) ───────────────────────────
	state.mAuthStatus = systray.AddMenuItem("Not signed in", "")
	state.mAuthStatus.Disable()
	state.mUploadStatus = systray.AddMenuItem("Last upload: never", "")
	state.mUploadStatus.Disable()

	systray.AddSeparator()

	// ── Watching controls ────────────────────────────────────────────────────
	mPause := systray.AddMenuItem("Pause watching", "Pause automatic uploads")
	mPause.Disable() // Phase 3
	state.mUploadNow = systray.AddMenuItem("Upload now", "Upload the watched file immediately")
	state.mUploadNow.Disable() // enabled after sign-in if watch paths are configured

	systray.AddSeparator()

	// ── History / config ─────────────────────────────────────────────────────
	mActivity := systray.AddMenuItem("View recent activity", "See recent upload history")
	mOpenConfig := systray.AddMenuItem("Open config folder", "Open the config folder")

	systray.AddSeparator()

	// ── Startup toggle ───────────────────────────────────────────────────────
	mStartWithOS := systray.AddMenuItem("Start with Windows", "Launch automatically at login")
	if a.Config.Behavior.StartWithOS {
		mStartWithOS.Check()
	}

	systray.AddSeparator()

	// ── Auth / meta ──────────────────────────────────────────────────────────
	state.mSignIn = systray.AddMenuItem("Sign in to Journal", "Authenticate with your Journal account")
	state.mSignOut = systray.AddMenuItem("Sign out", "Remove stored credentials")
	state.mSignOut.Hide()
	mAbout := systray.AddMenuItem("About Journal Companion", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Journal Companion")

	// Restore auth state from keychain (persisted across restarts).
	a.loadStoredCredentials()

	go a.handleEvents(mActivity, mOpenConfig, mStartWithOS, mAbout, mQuit)
}

// loadStoredCredentials reads the keychain on startup and updates the tray
// to the connected state if a token is present.
func (a *App) loadStoredCredentials() {
	token, _ := auth.LoadToken()
	if token == "" {
		return
	}
	username, _ := auth.LoadUsername()
	a.setConnected(token, username)
}

// setConnected updates all auth-dependent UI elements to the signed-in state.
// Safe to call from any goroutine.
func (a *App) setConnected(token, username string) {
	state.mu.Lock()
	state.token = token
	state.username = username
	state.mu.Unlock()

	label := "Connected to Journal"
	if username != "" {
		label = "Connected as @" + username
	}
	state.mAuthStatus.SetTitle(label)
	systray.SetTooltip(label)
	state.mSignIn.Hide()
	state.mSignOut.Show()

	if len(a.Config.Watch) > 0 {
		state.mUploadNow.Enable()
	}
}

// setDisconnected returns the tray to the signed-out state.
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

// setUploadStatus updates the "Last upload" status line.
func (a *App) setUploadStatus(summary string) {
	state.mUploadStatus.SetTitle("Last upload: " + summary)
}

func (a *App) handleEvents(
	mActivity, mOpenConfig, mStartWithOS, mAbout, mQuit *systray.MenuItem,
) {
	for {
		select {
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

// startAuthFlow runs the browser-based device-token flow in a goroutine.
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

// signOut clears all stored credentials and returns the tray to disconnected state.
func (a *App) signOut() {
	if err := auth.DeleteCredentials(); err != nil {
		slog.Error("delete credentials", "err", err)
	}
	a.setDisconnected()
	slog.Info("signed out")
}

// uploadNow runs an upload for every configured watch path.
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

	client := uploader.New(a.Config.Network.APIBase, a.Version)

	var lastSummary string
	for _, w := range a.Config.Watch {
		result, err := client.Upload(token, w.Label, w.Path, a.DB)
		if err != nil {
			slog.Error("upload", "label", w.Label, "path", w.Path, "err", err)
			if result.Status == "network_error" {
				a.setUploadStatus("Upload pending — check connection")
			}
			// 401 means token revoked; drop back to disconnected.
			if err.Error() == "token rejected — sign in again" {
				a.setDisconnected()
				return
			}
			continue
		}
		lastSummary = result.Summary
	}

	if lastSummary != "" {
		ts := time.Now().Format("3:04 PM")
		a.setUploadStatus(fmt.Sprintf("%s — %s", ts, lastSummary))
	}
}

// showRecentActivity logs the last 10 uploads to the log file.
// Phase 2: no window UI; a dedicated activity window is Phase 3+.
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
	slog.Info("exiting cleanly")
}
