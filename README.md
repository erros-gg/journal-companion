# Journal Companion

A small Windows background application that watches your ESO SavedVariables
file and uploads it to [Journal](https://journal.erros.gg) automatically. No
window, no manual step — install once, then it handles uploads whenever ESO
writes a new file.

---

## Install

The fastest way is the one-line installer. Run this in PowerShell:

```powershell
irm https://journal.erros.gg/companion/install.ps1 | iex
```

This downloads the latest release from GitHub, verifies the SHA-256 checksum,
installs to `%LOCALAPPDATA%\JournalCompanion\`, and creates a Start Menu shortcut.
No admin rights required.

**Manual download:** if you prefer not to pipe a script, grab
`journal-companion.exe` from the
[latest release](https://github.com/erros-gg/journal-companion/releases/latest)
and put it anywhere you like. Double-click to run.

Windows will show a SmartScreen warning on first launch — the binary is not
code-signed in v1. Click **More info → Run anyway** to proceed. This is a
one-time prompt per install.

---

## Setup

1. After launching, a small leaf icon appears in your system tray.
2. Right-click it and choose **Sign in to Journal**. Your browser opens to
   journal.erros.gg, where you approve the connection.
3. The companion is now linked to your account and will upload automatically.

The config file lives at `%APPDATA%\Journal Companion\config.toml`. On first
run it is created with a commented-out ESO watch block — uncomment it and set
the path to your `Journal.lua` SavedVariables file. Right-click the tray icon
and choose **Open config folder** to get there quickly.

---

## What it does

- Watches your `Journal.lua` SavedVariables file for changes
- Compresses and uploads it to Journal whenever ESO writes a new version
  (on logout, `/reloadui`, or quit to title)
- Polls until the upload is parsed, then updates the tray with the outcome
- Downloads the Journal price snapshot to your addon folder so in-game price
  lookups work without opening a browser
- Queues uploads when offline and retries when connectivity returns
- Persists sign-in across restarts using Windows Credential Manager

The companion is game-agnostic — it ships bytes to the server, which does all
parsing. No game knowledge is baked into this binary.

---

## Build from source

Requires Go 1.22 or later.

```powershell
# Using the build script
.\build.ps1

# Or directly
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -ldflags="-H=windowsgui -s -w -X main.version=dev" -o journal-companion.exe .
```

The `-H=windowsgui` linker flag suppresses the console window. Omit it when
building for debugging.

---

## Releases

Releases are cut by pushing a `v*` tag. The GitHub Actions workflow builds the
binary, verifies the size limit, generates the checksum, and publishes a GitHub
release with both files attached.

See [docs/distribution.md](docs/distribution.md) for the full release process.

---

## License

The Companion's source is published here for transparency and review. It is
not open source — all rights are reserved. See [LICENSE](./LICENSE) for the
full terms. You're welcome to read the code and file issues; forking,
modifying, or redistributing is not permitted.
