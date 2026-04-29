package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/erros-gg/journal-companion/internal/config"
	"github.com/erros-gg/journal-companion/internal/db"
	"github.com/erros-gg/journal-companion/internal/platform"
	"github.com/erros-gg/journal-companion/internal/tray"
)

// version is set at build time: -ldflags="-X main.version=x.y.z"
var version = "dev"

func main() {
	appDir, err := platform.ConfigDir()
	if err != nil {
		// No app dir means we can't log either; fall back to stderr and exit.
		slog.Error("resolve config dir", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(appDir, 0700); err != nil {
		slog.Error("create config dir", "path", appDir, "err", err)
		os.Exit(1)
	}

	setupLogging(appDir)
	slog.Info("Journal Companion starting", "version", version, "appDir", appDir)

	cfgPath := filepath.Join(appDir, "config.toml")
	cfg, err := config.LoadOrCreate(cfgPath)
	if err != nil {
		slog.Error("load config", "path", cfgPath, "err", err)
		os.Exit(1)
	}

	database, err := db.Open(filepath.Join(appDir, "companion.db"))
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	tray.Run(&tray.App{
		Version:    version,
		Config:     cfg,
		ConfigPath: cfgPath,
		DB:         database,
		AppDir:     appDir,
	})
}

// setupLogging redirects slog output to a rotating log file in appDir.
// Falls back silently to stderr on any error (better than crashing at startup).
func setupLogging(appDir string) {
	logPath := filepath.Join(appDir, "companion.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
}
