// Package tray owns the system tray icon and menu — the app's complete UI surface.
package tray

import (
	"log/slog"

	"github.com/erros-gg/journal-companion/internal/config"
	"github.com/erros-gg/journal-companion/internal/db"
	"github.com/erros-gg/journal-companion/internal/platform"
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

// Run starts the system tray and blocks until the user chooses Quit.
// It must be called on the main goroutine (systray requirement on some platforms).
func Run(app *App) {
	systray.Run(app.onReady, app.onExit)
}

func (a *App) onReady() {
	systray.SetIcon(iconDefault())
	systray.SetTooltip("Journal Companion — not signed in")

	// ── Status lines (disabled; read-only display) ──────────────────────────
	mAuthStatus := systray.AddMenuItem("Not signed in", "")
	mAuthStatus.Disable()
	mUploadStatus := systray.AddMenuItem("Last upload: never", "")
	mUploadStatus.Disable()

	systray.AddSeparator()

	// ── Watching controls ───────────────────────────────────────────────────
	// Disabled until Phase 3 (file watcher) and Phase 2 (auth) respectively.
	mPause := systray.AddMenuItem("Pause watching", "Pause automatic uploads")
	mPause.Disable()
	mUploadNow := systray.AddMenuItem("Upload now", "Upload the watched file immediately")
	mUploadNow.Disable()

	systray.AddSeparator()

	// ── History / config ────────────────────────────────────────────────────
	mActivity := systray.AddMenuItem("View recent activity", "See recent upload history")
	mOpenConfig := systray.AddMenuItem("Open config folder", "Open the config folder")

	systray.AddSeparator()

	// ── Startup toggle ──────────────────────────────────────────────────────
	mStartWithOS := systray.AddMenuItem("Start with Windows", "Launch automatically at login")
	if a.Config.Behavior.StartWithOS {
		mStartWithOS.Check()
	}

	systray.AddSeparator()

	// ── Auth / meta ─────────────────────────────────────────────────────────
	mSignIn := systray.AddMenuItem("Sign in to Journal", "Authenticate with your Journal account")
	mAbout := systray.AddMenuItem("About Journal Companion", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Journal Companion")

	go a.handleEvents(mActivity, mOpenConfig, mStartWithOS, mSignIn, mAbout, mQuit)
}

func (a *App) handleEvents(
	mActivity, mOpenConfig, mStartWithOS, mSignIn, mAbout, mQuit *systray.MenuItem,
) {
	for {
		select {
		case <-mOpenConfig.ClickedCh:
			if err := platform.OpenFolder(a.AppDir); err != nil {
				slog.Error("open config folder", "err", err)
			}

		case <-mStartWithOS.ClickedCh:
			// TODO Phase 4: wire up platform.SetStartWithOS.
			// For now, toggle the visual state and log; don't persist or touch the registry.
			slog.Info("TODO: implement Start with OS")

		case <-mActivity.ClickedCh:
			// TODO Phase 2/3: open a small activity window or log recent uploads.
			slog.Info("TODO: implement View recent activity")

		case <-mSignIn.ClickedCh:
			// TODO Phase 2: launch the browser-based device-token auth flow.
			slog.Info("TODO: implement Sign in (Phase 2)")

		case <-mAbout.ClickedCh:
			slog.Info("TODO: implement About", "version", a.Version)

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) onExit() {
	slog.Info("exiting cleanly")
}
