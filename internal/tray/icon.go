package tray

import "github.com/erros-gg/journal-companion/assets"

// iconDefault returns the base tray icon bytes.
// In Phase 3+ this will return different icons based on watcher/upload state
// (e.g. an animated upload indicator, an error overlay).
func iconDefault() []byte {
	return assets.Icon
}
