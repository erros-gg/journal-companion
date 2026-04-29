# Phase 2 Completion Summary

## What was built

### `internal/auth/keyring.go`
`SaveToken`, `LoadToken`, `SaveUsername`, `LoadUsername`, `DeleteCredentials` using `go-keyring` (Windows Credential Manager backend).

### `internal/auth/flow.go`
`StartFlow(apiBase, deviceLabel)` blocks until success, timeout, or error:
- Generates a 16-byte random hex session ID
- Starts a localhost HTTP server (port 53219 first, OS-assigned fallback)
- `POST /api/companion/auth/initiate` → gets browser URL
- Opens browser via `platform.OpenBrowser`
- Polls `POST /api/companion/auth/poll` every 2s concurrently with the localhost callback listener
- Whichever arrives first (redirect or poll) wins via a channel
- Stores token + username in keychain; shuts down the listener deterministically

### `internal/uploader/compress.go`
`CompressFile` single-pass: hashes original bytes while gzip-compressing, returns compressed bytes + original size + `sha256:<hex>` string.

### `internal/uploader/client.go`
`Client.Upload(token, label, path, db)`:
- Sends all spec-required headers (`X-Companion-Label`, `X-Companion-Filename`, `X-Companion-Filesize-Original`, `X-Companion-Filehash-Original`, `X-Companion-Companion-Version`, `X-Companion-OS`)
- On 202: records in SQLite via `RecordUpload`
- Polls every 2s for first 30s, then 10s up to 5-minute timeout
- On terminal status: calls `UpdateUploadResult`
- On network error: records a `pending` row; surfaces "Upload pending — check connection"
- 401 response surfaces as "token rejected — sign in again" (tray handles sign-out)

### `internal/tray/tray.go`
Full rewrite with live state:
- Loads keychain credentials at startup; tray reflects persisted sign-in across restarts
- "Sign in" / "Sign out" items toggle via `Hide()`/`Show()`
- "Upload now" enabled only when signed in AND `config.Watch` is non-empty
- "Signing in…" and "uploading…" transient status during async ops
- "Last upload" line updated with timestamp + result summary after each upload
- 401 from upload automatically signs the user out

### `internal/platform/`
`OpenBrowser` added to all three files (`rundll32` on Windows, `open` on macOS, `xdg-open` on Linux). `OSName()` added. `SetStartWithOS` now returns `ErrNotImplemented` on all platforms.

### `internal/watcher/watcher.go`
`ErrNotImplemented` defined; comment updated.

---

## Decisions not dictated by the spec

1. **Poll vs. callback — race-to-first**: the spec describes both a poll endpoint and a localhost callback. Both run concurrently; whichever delivers the token first wins. This is more robust than choosing one.

2. **Pending upload ID format**: when a network error prevents a server upload, the local DB row gets ID `pending_<unix_nano>`. This is a placeholder that lets the user see the failure without crashing. Phase 4 retry will need to handle these specially.

3. **Username storage**: the spec says the tray shows "Connected as @erros" but doesn't define where the username comes from post-restart. It is stored as a second keychain entry alongside the token.

4. **`X-Companion-OS` value**: the spec shows `windows-11` in the example but doesn't define a format. Uses `runtime.GOOS` (`"windows"`, `"darwin"`, `"linux"`) — simple, accurate, and won't break cross-compilation. OS version detail can be added in Phase 4.

5. **Device label**: hardcoded to `"Companion App on Windows"` for Phase 2. Phase 4 should derive it from the actual OS or make it user-configurable.

6. **"View recent activity"**: the spec shows this as a tray menu item but gives no Phase 2 behaviour. Logs the last 10 uploads to `companion.log`. A proper UI window is deferred to Phase 3+.

---

## Spec ambiguities resolved by judgment

- **`/api/companion/auth/initiate` and `/api/companion/auth/poll` request/response shapes** are not fully spelled out. Defined as documented in the `flow.go` package comment; the server-side implementation needs to match this contract.
- **Polling timeout scope**: the spec says "5-minute total timeout." A single `context.WithTimeout` is shared by both the poll goroutine and the localhost listener, so the clock runs from the moment `StartFlow` is called regardless of which path delivers the token.

---

## Build results

| | |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `gofmt -l .` | clean |
| Binary size | 9.9 MB (limit: 20 MB) |
