package syncer

import (
	"errors"
	"path/filepath"

	"github.com/erros-gg/journal-companion/internal/config"
)

// DeriveSavedVarsDir finds the watch entry labelled "eso-savedvariables" and
// returns its parent directory — the ESO SavedVariables folder.
//
// Returns an error if no such entry is configured.
func DeriveSavedVarsDir(watches []config.Watch) (string, error) {
	for _, w := range watches {
		if w.Label == "eso-savedvariables" {
			return filepath.Dir(w.Path), nil
		}
	}
	return "", errors.New("no eso-savedvariables watch configured")
}

// DeriveAddonDir returns the Journal addon directory by walking up from
// SavedVariables to the ESO live root, then into AddOns.
//
// ESO's SavedVariables live at:  .../live/SavedVariables/
// The addon lives at:            .../live/AddOns/Journal/
//
// JournalPrices.lua is written here — not to SavedVariables — so ESO never
// serialises and overwrites it when the player does /reloadui.
func DeriveAddonDir(watches []config.Watch) (string, error) {
	savedVarsDir, err := DeriveSavedVarsDir(watches)
	if err != nil {
		return "", err
	}
	liveDir := filepath.Dir(savedVarsDir)
	return filepath.Join(liveDir, "AddOns", "Journal"), nil
}
