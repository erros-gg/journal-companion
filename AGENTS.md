<!-- BEGIN:powershell-context -->
# This is a PowerShell-based desktop sync agent

The repo is the optional desktop companion for Journal — a PowerShell script that watches the ESO SavedVariables file and syncs uploads to the Journal API in the background. Most users don't need it; it exists for power users who want background sync without manual `/journal upload` in-game.

## Conventions

- **Language:** PowerShell. Windows-first. Cross-platform PowerShell Core support is a nice-to-have, not a requirement.
- **Distribution:** raw PowerShell from a public GitHub release (currently). Eventually a signed installer, but that's deferred.
- **Configuration:** TOML config file alongside the script. The user edits `[[watch]]` blocks to point at their ESO SavedVariables path.
- **No npm, no node_modules.** This is not a JavaScript project. Don't suggest adding `package.json` or similar.
<!-- END:powershell-context -->

<!-- BEGIN:phase-context -->
# This is the optional step 5 of onboarding

In the canonical five-step user journey (Discover → Sign up → Install addon → Upload first sales → Join The Study), the companion is the optional step 6 — positioned as a power-user upgrade after the basic loop works. Don't promote it as core to using Journal.

When working on this repo's README, marketing copy, or any user-facing text:

- Lead with: "This is optional. Skip if you're new to PowerShell."
- Be honest about the friction: "This is a script from GitHub. Here's what it does, here's how to verify the source before running."
- Don't hide the rough edges; document them.

## Pending: SavedVariables auto-detect

Phase 4 of the companion roadmap (filed in the project's Notion ideas) includes a first-run experience improvement: auto-detect the standard ESO SavedVariables path (`%USERPROFILE%\Documents\Elder Scrolls Online\live\SavedVariables\JournalCompanion.lua`). If detected, write a complete `[[watch]]` block in the default config; if not detected, write a commented-out example with placeholder values.

This is a real follow-up, not a hypothetical. When the user asks about Phase 4 work or the first-run experience, this is what they mean.
<!-- END:phase-context -->

<!-- BEGIN:brand-context -->
# User-facing strings and the README follow the brand

Everything a user sees from this companion — the README, console output, error messages, config file comments — must match the Erros voice. The brand specifications live in a sibling repository at `../erros-brand/` (assuming standard side-by-side checkout under `F:\src\erros.gg\`).

Before writing or modifying any user-facing text, you MUST read:

- `../erros-brand/01-brand-foundation/guidelines/voice-and-tone.md` — voice rules with examples
- `../erros-brand/01-brand-foundation/guidelines/erros-brand-guidelines.md` — the master brand spec

You must do this even if you've read these files earlier in this session. Re-reading is cheap; voice drift is expensive.

If `../erros-brand/` is not present in the local checkout, STOP and tell the user. Do not invent brand decisions.

## Companion-specific voice notes

PowerShell output to the console can be terse but should not be robotic. Errors should explain what went wrong and what the user should try, not just report a stack trace. Config file comments should be friendly to users editing them by hand.

Keep "A daybook for the trade." OUT of the companion's user-facing surfaces. The tagline is reserved for hero/marketing surfaces; a desktop sync agent's console isn't one of them.
<!-- END:brand-context -->