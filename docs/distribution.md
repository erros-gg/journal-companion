# Distribution

How releases are built, published, and delivered to users.

---

## End-to-end flow

```
User visits journal.erros.gg/companion
  └─► page instructs: irm https://journal.erros.gg/companion/install.ps1 | iex
        └─► Next.js route handler at /companion/install.ps1
              └─► serves raw content from scripts/Install.ps1 in this repo
                    └─► script hits api.github.com/repos/erros-gg/journal-companion/releases/latest
                          └─► downloads journal-companion.exe + journal-companion.exe.sha256
                                └─► verifies SHA-256, installs, launches
```

The companion page and the route handler that serves the script live in the `erros-gg/journal` repo. This repo (the companion) owns the install script source and the release artifacts. The journal repo serves the script at a stable URL so users don't have a raw.githubusercontent.com link in their muscle memory.

---

## What GitHub Actions produces

The workflow at `.github/workflows/release.yml` triggers on any tag push matching `v*`. It:

1. Cross-compiles a Windows binary from an Ubuntu runner (`GOOS=windows GOARCH=amd64 CGO_ENABLED=0`)
2. Injects the tag name as the version string via `-ldflags="-X main.version=<tag>"`
3. Verifies the binary is under 20 MiB (the spec's hard limit)
4. Runs `sha256sum journal-companion.exe > journal-companion.exe.sha256`
5. Extracts the release notes body from the first `## [x.y.z]` section in `CHANGELOG.md`
6. Publishes a GitHub release with both files attached

The `.sha256` file follows `sha256sum` output format: one line, the 64-char lowercase hex hash, two spaces, then the filename. The install script parses the first whitespace-delimited token of the first line — this format satisfies that expectation exactly.

Tags containing a hyphen (e.g., `v0.2.0-rc1`) are published as pre-releases. Tags without a hyphen are published as stable releases.

---

## Cutting a release

1. Update `CHANGELOG.md` — add a new `## [x.y.z] - YYYY-MM-DD` section above the previous one. The workflow uses this for the release body.

2. Commit and push to `master`:
   ```
   git add CHANGELOG.md
   git commit -m "chore: bump changelog for vX.Y.Z"
   git push origin master
   ```

3. Tag the commit and push the tag:
   ```
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

4. Watch the Actions tab. The workflow runs automatically. When it completes, the release is live at `github.com/erros-gg/journal-companion/releases/tag/vX.Y.Z`.

5. Verify the release page shows both assets (`journal-companion.exe` and `journal-companion.exe.sha256`) and that the release notes body looks correct.

That's it. The install script always resolves `latest` by default, so existing users running the script again will get the new version.

---

## Release notes convention

`CHANGELOG.md` follows the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Each release has a `## [x.y.z] - YYYY-MM-DD` heading followed by categorised changes under `### Added`, `### Changed`, `### Fixed`, `### Removed`.

The workflow extracts everything between the first `## [` heading and the second one. Keep the body focused — users running the install script see this as the GitHub release description.

An `## [Unreleased]` section lives at the top for collecting changes that haven't been tagged yet. Move its contents into a new versioned section when cutting a release.

---

## How the install script works

The script (`scripts/Install.ps1`) is a self-contained PowerShell installer that requires no admin rights.

1. Hits the GitHub releases API to resolve the target version (latest by default, or a specific tag via `-Version`)
2. Finds the `journal-companion.exe` and `journal-companion.exe.sha256` assets
3. Downloads both to a temp directory
4. Verifies the SHA-256 hash — aborts if the hash doesn't match
5. Stops any running companion process
6. Moves the verified binary to `%LOCALAPPDATA%\JournalCompanion\journal-companion.exe`
7. Creates a Start Menu shortcut
8. Registers autostart under `HKCU\...\Run` (skippable with `-NoAutostart`)
9. Launches the binary (skippable with `-NoLaunch`)

The checksum verification step is unconditional — the script will refuse to install a binary whose hash doesn't match the published checksum. This guards against network corruption; it is not a substitute for code signing.

On uninstall (`-Uninstall`), the script removes the binary, the Start Menu shortcut, and the autostart registry entry. User data (`config.toml`, `companion.db`, `companion.log`) at `%APPDATA%\Journal Companion\` is left intact. Users who want a fully clean uninstall can delete that folder manually.

---

## Hosting the install script

The install script lives at `scripts/Install.ps1` in this repo. The journal repo serves it at `https://journal.erros.gg/companion/install.ps1` via a route handler that fetches from `raw.githubusercontent.com/erros-gg/journal-companion/master/scripts/Install.ps1`.

Serving it through the journal domain rather than pointing users at a raw GitHub URL means:

- The URL is stable and human-readable
- Future changes to script hosting don't require updating every piece of documentation that links to it
- The journal server can add caching, logging, or a signature header without touching the companion repo

---

## Code signing

There is no code signing in v1. Users will see a Windows SmartScreen warning on first launch. The install script calls `Unblock-File` after installing, which removes the Zone.Identifier alternate data stream that triggers the second SmartScreen prompt on every subsequent launch. The initial prompt on first launch is accepted once.

When code signing is added:

- The workflow gains a signing step that runs after the build, using a certificate loaded from a GitHub Actions secret
- Both the `.exe` and the `.sha256` file are published as before; the `.sha256` is still the integrity check for the download, and the Authenticode signature is the code-trust check
- The SmartScreen prompt disappears for signed releases
- The install script's verification step does not change — it still checks SHA-256; the Authenticode check happens at the OS level automatically
- The companion page's "first launch" note about SmartScreen can be removed

---

## Building locally

```powershell
.\build.ps1
# Output: dist\journal-companion.exe

.\build.ps1 -Version v0.2.0
# Output: dist\journal-companion.exe (with version v0.2.0 embedded)
```

Or directly with Go:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-H=windowsgui -s -w -X main.version=$(git describe --tags)" \
  -o journal-companion.exe .
```

The `-H=windowsgui` flag suppresses the console window that would otherwise appear when the binary is double-clicked. Omit it if you want a console window for debugging.
