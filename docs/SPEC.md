# Journal Companion + Ingestion Pipeline — Technical Specification

This document specifies two coordinated systems:

1. **Journal Companion** — a small Go application that watches user-configured files for changes and uploads them to Journal.
2. **Journal Ingestion Pipeline** — server-side infrastructure in the Journal repo that receives uploads, stores them, parses them, and writes observations to the database.

These are designed together because the contract between them is the most important interface in the system. The companion is deliberately game-agnostic; all game-specific logic (Lua parsing, ESO schema knowledge, etc.) lives in the server.

The primary user for v1 is Erros (project author), whose workflow is: scan guild traders on a dedicated character throughout the day while developing Journal on a separate machine. The companion eliminates manual upload from that loop. v2 expansion is to other contributors and other games; the architecture is built to support both.

---

## Part 1: Companion Application

### Goals

**Primary:** Watch a configured file (or files) for changes and upload them to Journal automatically.

**Secondary:** Provide visibility into upload status through the system tray, without ever opening a window.

**Tertiary:** Stay game-agnostic. The companion should require no changes to support new games — only the addon and the server need to know game-specific details.

**Cost basis:** No code-signing certificate in v1. No installer. No hosted update infrastructure. Distribution is a single binary downloaded from a GitHub release.

**Non-goals for v1:** macOS and Linux support (architecture should accommodate them; they're priority for v1.x). Real-time streaming. Auto-update. Console platforms (don't apply to a desktop daemon).

### Language: Go 1.22+

Reasoning unchanged from prior spec — single static binary, minimal AV heuristic friction, no frontend pipeline, excellent cross-platform compilation. The architectural simplification of moving parsing to the server makes Go an even cleaner fit, since the companion's responsibilities are now purely: watch, compress, upload, retry.

### Architecture overview

```
┌────────────────────────────────────────────────────────────────┐
│  Single Go process                                             │
│                                                                │
│  ┌─────────────┐   on change   ┌──────────────┐               │
│  │   Watcher   │ ────────────► │   Uploader   │               │
│  │ (fsnotify)  │               │              │               │
│  └─────────────┘               │ - read file  │               │
│                                │ - gzip       │               │
│                                │ - POST       │               │
│                                │ - poll status│               │
│                                └──────┬───────┘               │
│                                       │                       │
│  ┌─────────────┐                      │                       │
│  │    Tray     │ ◄──────── status ────┘                       │
│  │ (systray)   │   updates                                    │
│  └──────┬──────┘                                              │
│         │                                                     │
│         ▼                                                     │
│  ┌─────────────────────────────────────────────────────┐     │
│  │  Auth (one-time, browser flow + localhost callback) │     │
│  └─────────────────────────────────────────────────────┘     │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  SQLite (upload history)                             │    │
│  │  TOML (settings, watched paths)                      │    │
│  │  Log file (slog)                                     │    │
│  │  OS keychain (device token)                          │    │
│  └─────────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────────┘
                               │
                               ▼
                        Journal API
```

The companion has three core subsystems:

- **Watcher.** fsnotify-based file watching, with debouncing and hash-based deduplication.
- **Uploader.** Reads the changed file, gzip-compresses, POSTs to Journal, polls for parse result, records outcome, updates tray.
- **Tray.** Renders the icon, builds the menu, surfaces status. The complete UI surface.

Plus a one-time auth subsystem and shared persistence (SQLite + TOML + keychain).

### File watching

Same approach as previously specced:

- `github.com/fsnotify/fsnotify` for cross-platform watching
- Watch the specific file(s) configured in TOML, not entire directories
- On WRITE event, compare mtime against last-seen value; if newer, hash the contents and compare against last-seen hash; if both new, queue for upload
- Debounce rapid events (within 500ms window) into a single upload
- Handle file locks gracefully — the file may be held by the source application; retry with backoff

The companion does not validate or inspect the file beyond hash-based change detection. It treats the file as opaque bytes.

### Configuration

`%APPDATA%\Journal Companion\config.toml` on Windows. Equivalents on macOS and Linux when those platforms ship.

```toml
[network]
api_base = "https://journal.erros.gg"

[behavior]
start_with_os = false
upload_on_change = true
debounce_ms = 500

# One or more watched files. Each watch has a label that's sent to the server
# so it can route the upload to the right parser.
[[watch]]
label = "eso-savedvariables"
path = "C:\\Users\\<user>\\Documents\\Elder Scrolls Online\\live\\SavedVariables\\Journal.lua"

# Future: a user playing multiple games would add more watches:
# [[watch]]
# label = "wow-savedvariables"
# path = "C:\\Program Files (x86)\\World of Warcraft\\..."
```

The `label` field is the contract the companion has with the server. The server uses the label to decide which parser to invoke. The companion has no opinion on what labels mean — it just sends them along with each upload.

The first-run experience: on initial launch, the companion writes a default config with no `[[watch]]` blocks. The user must add at least one before anything happens. The tray menu has "Open config folder" to make this easy. Eventually a small setup wizard could be added, but for v1 the manual config is acceptable for the technical audience.

### Upload payload

The companion sends each detected change as a single HTTP request:

```
POST /api/companion/upload
Authorization: Bearer <device-token>
Content-Type: application/octet-stream
Content-Encoding: gzip

X-Companion-Label: eso-savedvariables
X-Companion-Filename: Journal.lua
X-Companion-Filesize-Original: 487293
X-Companion-Filehash-Original: sha256:abc123...
X-Companion-Companion-Version: 1.0.3
X-Companion-OS: windows-11

<gzipped raw file contents>
```

The server responds immediately with a job ID:

```
202 Accepted
Content-Type: application/json

{
  "uploadId": "upload_8f3k2lqp9m4d",
  "statusUrl": "/api/companion/upload/upload_8f3k2lqp9m4d",
  "queuedAt": "2026-04-28T14:32:11Z"
}
```

The companion records `(uploadId, queuedAt, label, original_filehash)` in its local SQLite history table with status `pending`, then begins polling the status URL.

### Polling for parse result

After upload, the companion polls `GET /api/companion/upload/:id` with backoff: every 2 seconds for the first 30 seconds, then every 10 seconds for up to 5 minutes total. Most parses finish in under 10 seconds; the long timeout is for rare cases where the worker is backed up.

Response while still processing:

```json
{
  "uploadId": "upload_8f3k2lqp9m4d",
  "status": "queued" | "parsing",
  "queuedAt": "2026-04-28T14:32:11Z"
}
```

Response when complete:

```json
{
  "uploadId": "upload_8f3k2lqp9m4d",
  "status": "completed",
  "queuedAt": "2026-04-28T14:32:11Z",
  "completedAt": "2026-04-28T14:32:14Z",
  "result": {
    "observationsAccepted": 247,
    "observationsRejected": 3,
    "observationsDuplicate": 1842,
    "warnings": [
      "Skipped 3 observations with malformed timestamps"
    ]
  }
}
```

Response when failed:

```json
{
  "uploadId": "upload_8f3k2lqp9m4d",
  "status": "failed",
  "queuedAt": "2026-04-28T14:32:11Z",
  "completedAt": "2026-04-28T14:32:14Z",
  "error": {
    "code": "parse_error",
    "message": "Could not parse SavedVariables file: unexpected token at line 3,847",
    "userMessage": "The file appears corrupted. This may resolve on next ESO logout."
  }
}
```

The companion updates its local history with the outcome and surfaces a summary in the tray menu's "Last upload" line. `userMessage` is what gets shown to the user; `message` is logged for debugging.

### Authentication: device-token flow

Same as previously specced — browser-based device authorization with a localhost callback.

Brief recap of the flow (full detail in the previous spec):

1. User clicks "Sign in to Journal" in tray menu
2. Companion generates a session ID, starts a localhost HTTP server, calls `/api/companion/auth/initiate`
3. Browser opens to `https://journal.erros.gg/companion/authorize?session=...`
4. User signs in (via existing Supabase auth) if not already signed in
5. User clicks "Authorize" with an editable device label ("Companion App on Windows" by default)
6. Server stores a `device_tokens` row, redirects browser to localhost callback with the token
7. Companion stores the token in OS keychain, shuts down the local server
8. Tray updates to "Connected as @erros"

All subsequent API calls use `Authorization: Bearer <token>`.

Token revocation happens via Journal's settings page (`/dashboard/settings/devices`). Revoked tokens cause the companion's next call to receive 401, which surfaces as "Sign in again" in the tray.

### Tray UI

Same as previously specced. Right-click menu:

```
Connected as @erros                                   (header)
Last upload: 2 min ago — 247 observations             (header)
─────────────
Pause watching
Upload now
─────────────
View recent activity
Open config folder
─────────────
Start with Windows                                    (toggle)
─────────────
Sign out
About Journal Companion
Quit
```

Tooltip on hover summarizes connection and watch state. Icon state changes briefly during upload, gains a small overlay on persistent error.

### Cross-platform

Architecture commits to all desktop platforms long-term, ships Windows-only at v1.

The codebase is structured so that platform-specific code lives in clearly-isolated files (`platform_windows.go`, `platform_darwin.go`, `platform_linux.go` using Go's build tag conventions). Areas where this matters:

- **Tray icon library.** `getlantern/systray` works on all three desktop platforms but each has quirks. Test on each before committing.
- **Path resolution.** `%APPDATA%` on Windows, `~/Library/Application Support` on macOS, `~/.config` on Linux. Use `os.UserConfigDir()` from the standard library.
- **Start-with-OS implementation.** Registry on Windows, LaunchAgent plist on macOS, autostart desktop file on Linux. Three small platform-specific functions behind one interface.
- **Browser-open command.** `rundll32` on Windows, `open` on macOS, `xdg-open` on Linux.
- **App bundling.** Mac requires a `.app` bundle for proper menu bar behavior; Linux benefits from AppImage or similar packaging. v1 doesn't need to solve these.

Console platforms (Xbox, PlayStation) are explicitly out of scope for the companion. They don't run desktop applications. If Journal expands to console games, data ingestion for those games must happen through other means (in-game APIs, paired mobile apps, manual web upload) and is a separate architectural problem.

### Build phases

**Phase 1 — Skeleton (1-2 days).** Tray icon renders, menu skeleton, SQLite/TOML scaffolding, Quit works. Validates dependencies.

**Phase 2 — Auth and manual upload (2-3 days).** Device-token flow complete (browser, localhost callback, keyring storage). "Upload now" tray item picks the configured file, gzips, uploads, polls for result, records outcome.

**Phase 3 — File watching (2 days).** fsnotify watcher on configured paths. Hash-based change detection. Automatic upload on detected changes. Pause/Resume controls. Tray icon state changes.

**Phase 4 — Polish and reliability (2 days).** Offline queue with retry, robust handling of auth expiration and locked files, "Start with OS" implementation, version check against GitHub releases, logging cleanup.

**Phase 5 — Cross-platform expansion (deferred, post-v1).** Mac and Linux ports.

### Repository structure

`erros-gg/journal-companion`. Standard Go layout:

```
journal-companion/
├── README.md
├── LICENSE
├── go.mod
├── go.sum
├── main.go
├── internal/
│   ├── auth/
│   │   ├── flow.go
│   │   └── keyring.go
│   ├── watcher/
│   │   └── watcher.go
│   ├── uploader/
│   │   ├── client.go
│   │   ├── compress.go
│   │   └── retry.go
│   ├── tray/
│   │   ├── tray.go
│   │   └── icon.go
│   ├── config/
│   │   └── config.go
│   ├── db/
│   │   ├── db.go
│   │   └── queries.go
│   └── platform/
│       ├── windows.go      # build tag: //go:build windows
│       ├── darwin.go       # build tag: //go:build darwin
│       └── linux.go        # build tag: //go:build linux
├── assets/
│   └── icon.ico
├── scripts/
│   └── copy-icons.sh
└── .github/
    └── workflows/
        └── release.yml
```

### Build and release

```bash
go build -ldflags="-H=windowsgui -s -w -X main.version=$(git describe --tags)" -o journal-companion.exe .
```

GitHub Actions on tag push: build, hash, attach to release, with notes from CHANGELOG.md.

### Acceptance criteria for companion v1

- [ ] Single binary runs on fresh Windows 10/11
- [ ] Tray icon appears within 2 seconds of launch
- [ ] First-run device-token auth completes without copying or pasting strings
- [ ] App detects file changes, uploads automatically, polls for result, surfaces outcome
- [ ] Upload history visible via tray menu
- [ ] App handles offline gracefully — uploads queue and retry
- [ ] App's idle memory footprint under 30MB
- [ ] App's idle CPU usage under 0.5%
- [ ] Binary size under 20MB
- [ ] No code signing required (acceptable: SmartScreen warning on first launch)
- [ ] Codebase compiles cleanly with `GOOS=darwin` and `GOOS=linux` (even if untested at runtime)

---

## Part 2: Journal Ingestion Pipeline

The server side of the system. Receives compressed uploads from companion clients, stores them, parses them asynchronously, writes observations.

This is built in the `erros-gg/journal` repo as new endpoints, a new background worker, and new database tables. It coexists with all existing Journal functionality.

### Goals

**Primary:** Accept companion uploads, parse them reliably, ingest observations into the database.

**Secondary:** Make every parse error visible to the developer (Erros) for debugging without requiring user reports.

**Tertiary:** Keep raw uploads available for re-processing if parsing logic changes.

**Cost basis:** Use existing infrastructure (Supabase, Vercel). No new vendors, no new monthly costs.

### Architecture overview

```
Companion ──upload──► Next.js API route ──store──► Supabase Storage
                            │                           │
                            │                           │
                            └──enqueue──► Job queue ◄──read──┐
                                              │              │
                                              ▼              │
                                       Parse worker ────────┘
                                              │
                                              ▼
                                    Game-specific parser
                                    (eso, wow, ...)
                                              │
                                              ▼
                                       Supabase Postgres
                                       (observations, etc.)
                                              │
                                       Writes job result
                                              │
Companion ◄──poll──── Next.js API route ──read─┘
```

Three runtime components on the server side:

1. **Upload endpoint.** A Next.js API route. Receives the upload, validates auth, stores the raw file, writes a job row, returns the job ID.
2. **Parse worker.** Background process or scheduled function. Reads queued jobs, fetches the file, dispatches to the right parser based on label, writes results.
3. **Status endpoint.** A Next.js API route. Returns the current state of a job by ID.

### Database schema

New tables in your existing Supabase Postgres database:

```sql
-- Each upload received from a companion. Tracks lifecycle from upload through parse.
create table companion_uploads (
  id text primary key,                           -- "upload_8f3k2lqp9m4d"
  user_id uuid not null references auth.users(id),
  device_token_id uuid not null references device_tokens(id),

  label text not null,                           -- "eso-savedvariables", routes to parser
  original_filename text,
  original_filesize bigint,
  original_filehash text,                        -- sha256, for dedup detection
  companion_version text,
  companion_os text,

  storage_path text not null,                    -- path in Supabase Storage bucket
  storage_size bigint not null,                  -- compressed size, what we actually paid to store

  status text not null default 'queued',         -- queued | parsing | completed | failed
  queued_at timestamptz not null default now(),
  parse_started_at timestamptz,
  completed_at timestamptz,

  result jsonb,                                  -- {observationsAccepted, observationsRejected, ...} or null
  error jsonb,                                   -- {code, message, userMessage} or null

  -- For re-processing: when the parser version changes, we may want to re-parse old uploads
  parsed_with_parser_version text,
  reprocessed_from text references companion_uploads(id)
);

create index on companion_uploads (user_id, queued_at desc);
create index on companion_uploads (status) where status in ('queued', 'parsing');

-- Tracks active device tokens. One row per authorized companion install.
create table device_tokens (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references auth.users(id),
  token_hash text not null unique,               -- sha256 of the actual token; raw token never stored
  label text,                                    -- "Companion App on Windows" — user-editable
  created_at timestamptz not null default now(),
  last_used_at timestamptz,
  revoked_at timestamptz,
  -- Optional: track originating session_id from the auth flow for audit
  auth_session_id text
);

create index on device_tokens (user_id) where revoked_at is null;
create index on device_tokens (token_hash) where revoked_at is null;
```

Row-level security policies on both tables: users can only see their own uploads and their own device tokens. The parse worker uses the service-role client to bypass RLS for cross-user operations.

### Storage

Use Supabase Storage with a bucket named `companion-uploads`. Bucket is private — access goes through signed URLs only.

Storage path convention:

```
companion-uploads/{user_id}/{yyyy}/{mm}/{upload_id}.gz
```

This makes per-user lifecycle policies straightforward (delete a user's uploads if they revoke all their data, retain for N days for re-processing, etc.).

**Retention policy for v1:** keep uploads for 90 days, then a cron job deletes anything older where the corresponding `companion_uploads` row has `status = 'completed'`. Failed uploads kept indefinitely until investigated. This balances re-processability against cost.

At v1 scale (Erros + maybe 10 guildies), 90 days of uploads is negligible storage. As scale grows, the retention can shorten.

### Endpoints

**`POST /api/companion/upload`**

Receives a gzipped file. Auth via bearer device token.

Validates:
- Auth token is valid and not revoked (look up by hash in `device_tokens`)
- `X-Companion-Label` header is present and matches a known parser
- Content-Encoding is `gzip`
- File size (after decompression check or Content-Length) is reasonable — reject anything over, say, 50MB to prevent abuse

Behavior:
- Generate a new `upload_id` (`upload_` + base32-encoded random bytes)
- Stream the request body (still compressed) directly to Supabase Storage at the conventional path
- Insert a row into `companion_uploads` with status `queued` and all metadata
- Update `device_tokens.last_used_at`
- Enqueue a parse job (see queue section below)
- Return 202 with the upload ID and status URL

This endpoint should respond in well under a second — it's just storing the file and writing two database rows. Heavy work is deferred to the worker.

**`GET /api/companion/upload/:id`**

Returns the current status of an upload. Auth via bearer device token. RLS ensures the user only sees their own uploads.

Returns the row from `companion_uploads`, formatted as the JSON shape described in Part 1's "Polling for parse result" section.

**`POST /api/companion/auth/initiate`**, **`POST /api/companion/auth/poll`**, **`GET /api/companion/auth/authorize`** (and the associated browser-facing page at `/companion/authorize`)

The device-token auth flow endpoints. Detailed in the previous spec; nothing changes from the move to server-side parsing.

**`GET /api/companion/version`** *(optional, v1.5)*

Returns the latest companion version. Used by companion clients to check whether an update is available. Reads from a static table or constant; updated when a new release is published.

### Parse worker

The component most needs careful thought because it's the system's heart.

**Triggering.** Two viable patterns:

**Option A: Vercel Cron.** A cron job runs every minute, pulls any `queued` jobs, parses them. Simple but introduces up to 60 seconds of latency between upload and parse start. Acceptable for v1 — users won't notice the difference between 15s and 60s for parse-and-respond on a logout-triggered upload.

**Option B: Supabase Realtime + Edge Functions.** When a row is inserted into `companion_uploads`, a Supabase trigger fires an Edge Function that does the parse. Lower latency, more sophisticated. More moving pieces to debug.

**Recommendation: Option A for v1.** Boring is better. When you have evidence that the latency matters, switch to B.

**Job processing flow:**

```
1. Atomically claim a queued job (UPDATE ... WHERE status = 'queued' RETURNING)
   Set status = 'parsing', parse_started_at = now()

2. Fetch the file from Supabase Storage using the storage_path

3. Decompress (gzip)

4. Look up the parser registered for the upload's label
   If unknown label: write error result, status = 'failed', return

5. Invoke the parser. Returns { observations, warnings, errors }

6. For each observation:
   - Validate against schema
   - Check for duplicates against existing data (by some game-specific dedup key)
   - Insert if valid and new
   - Track per-observation outcomes

7. Write the aggregate result to companion_uploads.result
   Set status = 'completed', completed_at = now()

8. On any error during steps 3-7:
   Capture the error context (which step, what data, full stack trace if available)
   Write to companion_uploads.error
   Set status = 'failed', completed_at = now()
   Log to your error monitoring service
```

The atomic claim in step 1 is important because cron jobs can overlap if a parse runs longer than the cron interval.

**Parser interface.**

The server-side parser registry is where game knowledge lives:

```typescript
// /lib/companion/parsers/index.ts
import { parseEsoSavedVariables } from "./eso";

export interface ParserResult {
  observations: Observation[];
  warnings: string[];
  errors: ParserError[];
  metadata: Record<string, unknown>; // game-specific metadata, e.g. account name
}

export interface Parser {
  label: string;
  version: string;
  parse(content: Buffer): Promise<ParserResult>;
}

const PARSERS: Record<string, Parser> = {
  "eso-savedvariables": {
    label: "eso-savedvariables",
    version: "1.0.0",
    parse: parseEsoSavedVariables,
  },
  // Future:
  // "wow-savedvariables": { ... },
};

export function getParser(label: string): Parser | null {
  return PARSERS[label] ?? null;
}
```

The ESO parser itself uses a Lua parsing library (Node has options like `luaparse` for AST-based parsing, or `fengari-web` for a full Lua interpreter). The choice of library is a v1 implementation detail; both work.

**The parser version field matters.** When you update the ESO parser to handle a new addon schema or fix a bug, you bump the version. The `companion_uploads.parsed_with_parser_version` records which version processed each upload. This enables: "find all uploads parsed with version <1.2.0 and re-parse them with the current version" — exactly the kind of operation that justifies storing raw files.

### Re-processing capability

Build this from day one, even if it's just an admin endpoint with no UI.

**`POST /api/admin/companion/reprocess`** *(admin-only, behind your existing admin auth)*

Body: `{ uploadIds?: string[], filterParserVersion?: string, label?: string }`

Behavior: for each matching upload, create a new `companion_uploads` row with `reprocessed_from` set to the original ID, status `queued`, same storage_path. The cron worker picks it up and re-parses. The new row gets the new parser version.

This means you never destroy data: original parse results are preserved alongside re-parsed results. For analytics, you can always query "the most recent parse result per upload."

For v1, this is a one-line admin tool — paste a list of upload IDs, kick off re-processing. UI for it is a v2 nicety.

### Observability

The whole point of server-side parsing is that you, the developer, can see what's happening. Make this easy:

**An admin dashboard view at `/admin/companion`** with:

- Recent uploads, with status, user, label, timing
- Filter by status (so you can find all `failed` uploads quickly)
- Drill into a single upload to see the error details, the raw file (downloadable), the parse output
- Aggregate stats: uploads per day, parse success rate, p95 parse duration, observations per upload

This is genuinely v1 work, not nice-to-have. Without it, you're flying blind. With it, you can see "8 uploads failed today, all with the same parse error at line 3,847" and immediately know what to fix.

**Logging.** Every parse failure logs the upload ID, user ID, error class, and error message. Consider integrating with whatever error monitoring you use elsewhere in Journal (Sentry, native Vercel logs, whatever).

### Coordination with the addon

The addon (`erros-gg/journal-addon`) writes the SavedVariables file that the companion ships. The addon's data schema is the parser's input contract.

This means the addon, the parser, and the companion together form a versioned system:

- Addon version 1.0 writes schema A
- Parser version 1.0 understands schema A
- Companion just ships bytes, version-independent

When the addon evolves to schema B:

- New parser version 2.0 understands both A and B (best practice — handle version drift gracefully)
- Old companion installs keep working — they ship the new file format, the new parser handles it
- Schema migration is a server-side concern, not a user-side concern

This is why server-side parsing is the right architecture: schema evolution doesn't require re-distributing the companion.

### Acceptance criteria for ingestion v1

- [ ] Companion can authenticate via device token and upload a file
- [ ] Upload endpoint accepts gzipped payloads, stores to Supabase Storage, returns job ID in <500ms
- [ ] Cron job picks up queued jobs within 60s
- [ ] ESO parser successfully parses real SavedVariables files from Erros's gameplay
- [ ] Parse failures produce useful error info accessible via the admin dashboard
- [ ] Observations from successful parses appear in Journal's existing observation tables
- [ ] Status endpoint returns accurate state for queued, parsing, completed, and failed jobs
- [ ] 90-day retention policy implemented as a daily cron job
- [ ] Admin reprocess endpoint exists (UI optional)
- [ ] Admin dashboard view shows recent uploads and lets developer drill into failures

---

## Part 3: Phase coordination

The two systems must be built in the right order. Suggested sequencing:

**Week 1 — Foundation in Journal repo:**
- Database tables (`device_tokens`, `companion_uploads`)
- Auth endpoints (`/api/companion/auth/*`)
- The `/companion/authorize` browser page
- Upload endpoint (`POST /api/companion/upload`) — accepts files, stores them, marks queued
- Status endpoint (`GET /api/companion/upload/:id`) — returns row from DB

At end of week 1: server can receive uploads but doesn't parse them yet.

**Week 2 — Companion app skeleton (Phases 1-2):**
- Tray icon, menu, config loading
- Device-token auth flow against the new Journal endpoints
- Manual "Upload now" — picks file from config, gzips, uploads, polls

At end of week 2: companion can authenticate and upload manually. Files arrive on the server, sit unparsed.

**Week 3 — Parser worker:**
- ESO parser implementation
- Cron job to process queued uploads
- Admin dashboard view to see what's happening
- Reprocess endpoint

At end of week 3: end-to-end working pipeline. Erros can run companion, scan in-game, log out, see observations appear in Journal.

**Week 4 — Companion file watching (Phase 3) and polish (Phase 4):**
- fsnotify-based automatic uploads
- Offline queue and retry
- Tray icon error states
- Start-with-OS

At end of week 4: companion is genuinely background — install, auth once, forget. Production-ready for the trading guild rollout.

This is roughly 4 weeks of focused work. The phases interleave between the two repos, which means sequential development if it's one person doing both. If Claude Code handles companion-side work in parallel with Journal-side work being done by you, the calendar compresses.

---

## Notes for implementers

- The contract between the companion and the server is the upload payload format and the polling response format. Don't deviate from those without coordinating both sides.
- Server-side parser failures should log liberally. The whole reason for this architecture is to make debugging easy.
- Storage costs scale linearly with users and upload frequency. At v1 scale they're irrelevant; review when user count crosses 1,000.
- The `label` field on uploads is the extension point for future games. Treat it as a stable contract — old companions in the wild may use the original labels, so don't rename them.
- The parser's job is *only* to extract observations from a file. It should not write to other tables, should not call external services, should not have side effects beyond producing a `ParserResult`. Side effects belong in the worker that calls the parser, so they can be tested and traced independently.
- Do not log the contents of uploaded files. Log metadata, IDs, sizes, errors — never the parsed observations themselves or raw file contents. User data privacy applies even to observability.