# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.1.0] - 2026-05-01

### Added

- System tray icon with right-click menu: sign in, sign out, upload now, view recent activity, open config folder, start with Windows toggle, quit
- Device-token browser auth flow — opens journal.erros.gg in the system browser, stores token in Windows Credential Manager (no passwords stored in config files)
- Manual upload: reads the configured SavedVariables file, gzip-compresses it, POSTs to Journal, polls for parse result, surfaces outcome in the tray menu
- Automatic upload on file change via fsnotify — debounced at 500ms, hash-based deduplication to skip unchanged writes
- Price file sync — companion downloads the Journal price snapshot from the server and writes `JournalPrices.lua` into the addon directory for in-game price lookups
- Upload history in SQLite — persists across restarts; tray "View recent activity" shows the last ten uploads with status and timing
- TOML config at `%APPDATA%\Journal Companion\config.toml` — created on first run with a commented-out ESO watch block
- Log file at `%APPDATA%\Journal Companion\companion.log`
- Start-with-Windows registry entry via tray toggle (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`)
- Windows installer script at `scripts/Install.ps1` — downloads from GitHub releases, verifies SHA-256 checksum, installs to `%LOCALAPPDATA%\JournalCompanion\`, creates a Start Menu shortcut

### Architecture

- CGO-free build (`CGO_ENABLED=0`) — no C toolchain required, cross-compiles cleanly from Linux
- All credentials stored in OS keychain, never in config files
- Game-agnostic by design — the label field routes uploads to the right server-side parser; no game knowledge in the companion binary
