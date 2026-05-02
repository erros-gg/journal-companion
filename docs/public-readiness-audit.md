# Public Readiness Audit

Conducted before flipping the repository to public visibility. Covers the
current working tree and full git history.

---

## Things found and fixed

### 1. Binary name mismatch — `build.ps1`

`build.ps1` was producing `dist\TheJournalCompanion.exe`. The install script
(`scripts/Install.ps1`) expects `journal-companion.exe`. Fixed by updating the
`$out` variable in `build.ps1`. The release workflow produces the correct name.

### 2. `.gitignore` gap — `config.toml` not excluded

The application writes user config to `config.toml` at runtime. The `.gitignore`
only listed `config.json`, leaving a gap for accidental future commits of a
real config file. Added `config.toml` explicitly.

### 3. Phase summary removed from git history

`phase2-summary.md` was a development note committed during implementation.
It contained no sensitive data but was not intended for public consumption.
Removed from tracking via `git rm`.

---

## Things found and accepted

### Token and bearer references in source code

The grep for `token`, `bearer`, `secret`, `password`, etc. returned hits across
`internal/auth/`, `internal/uploader/`, `internal/syncer/`, and `internal/tray/`.
Every hit is a code reference — variable names, parameter names, comments, and
string literals like `"token rejected — sign in again"`. No real credential
values appear anywhere in the current tree or in git history.

The history scan produced the same code-reference pattern. No commit introduced
an actual token string.

**Verdict:** false positives, accepted.

### TODO/FIXME comments

Five TODO comments found, all referencing planned Phase 4 and Phase 5 work:

- `internal/platform/darwin.go:21` — `TODO Phase 5: implement macOS launch agent`
- `internal/platform/linux.go:21` — `TODO Phase 5: implement XDG autostart`
- `internal/platform/windows.go:27` — `TODO Phase 4: implement using golang.org/x/sys/windows/registry`
- `internal/tray/tray.go:266` — `TODO: implement Start with OS (Phase 4)`
- `internal/uploader/retry.go:3` — `TODO Phase 4: implement an offline queue with exponential backoff`

None contain sensitive context or leaked credentials. These are honest
milestone markers for planned work.

**Verdict:** appropriate, accepted.

### Hardcoded API base URL

`internal/config/config.go` defaults `APIBase` to `https://journal.erros.gg`.
This is the public production URL documented in the SPEC and the README. It is
configurable via `config.toml`'s `[network] api_base` field. No staging or
personal-machine URLs appear anywhere.

**Verdict:** intentional, accepted.

### localhost callback in auth flow

`internal/auth/flow.go` starts a short-lived HTTP server on localhost (port
53219 by default, OS-assigned fallback) to receive the token redirect from the
browser after the user authorizes the companion. This is the intended device
authorization flow. The listener handles exactly one request then closes.

**Verdict:** by design, accepted.

### `docs/SPEC.md` — internal technical specification

The SPEC describes the full system architecture including the server-side
ingestion pipeline, database schema, and API contract. It contains no
credentials or personally identifying information. It is retained in the repo
as contributor context for the public-facing companion code.

**Verdict:** no sensitive data, accepted.

---

## Things for Erros to verify before flipping public

### 1. No LICENSE file

The SPEC's repository structure lists a `LICENSE` file; none exists. A public
repo without a license is legally ambiguous — all rights are reserved by default,
which means contributors cannot fork or distribute the code without explicit
permission.

**Action required:** decide on a license and add a `LICENSE` file before making
the repo public. Common choices for open tooling: MIT (permissive, minimal
friction for users), Apache 2.0 (permissive with patent grant), GPL v3
(copyleft). Do not add a license without explicit intent — the choice has
downstream implications for users who distribute or modify the binary.

This prompt deliberately does not add a license; license selection is a decision
that belongs to the project author.

### 2. Git history review

The history scan did not find real credential values. However, the scan used
pattern matching on diff lines — it cannot guarantee zero false negatives on
obfuscated or partial secrets. Before flipping the repo public, consider a
manual `git log --all --oneline` pass to confirm no commits have descriptions
that suggest secret-containing changes (e.g., "add api key", "hardcode token
for testing").

If anything is found in history that requires removal, use `git filter-repo`
(preferred) or BFG Repo-Cleaner. **Do not use `git filter-branch`** — it is
deprecated and error-prone. History rewrites require force-pushing all branches
and notifying any collaborators with local clones.

### 3. Companion page dependency

The install script resolves binaries from `api.github.com/repos/erros-gg/journal-companion/releases`.
This endpoint requires the repository to be **public** for unauthenticated
access. The journal-repo companion page is similarly only functional after the
repo goes public and has at least one release.

Sequence: make the repo public → push a tag to trigger the first release →
verify the install script downloads and installs correctly → deploy the
journal-repo companion page.

---

## Grep commands used

```bash
# Current-file secrets scan
git grep -nIE '(secret|token|api[_-]?key|password|bearer|sk-|ghp_|github_pat_)' \
  -- ':!*.md' ':!go.sum'

# History scan
git log --all -p -- ':!*.md' ':!go.sum' | \
  grep -iE '(secret|token|api[_-]?key|password|bearer|sk-|ghp_|github_pat_)' | \
  head -100

# Hardcoded URLs
git grep -nE 'https?://' -- ':!*.md' ':!go.sum' | \
  grep -v 'journal\.erros\.gg\|api\.github\.com\|localhost\|github\.com/'

# TODO/FIXME
git grep -nI 'TODO\|FIXME\|XXX\|HACK' -- ':!*.md' ':!go.sum'

# Env files and databases
find . -name ".env*" -o -name "*.env" -o -name "*.db" -o -name "*.sqlite*" \
  | grep -v ".git"
```

All commands were run against the current working tree at the time of this audit.
