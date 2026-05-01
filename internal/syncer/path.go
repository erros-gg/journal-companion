package syncer

import (
	"errors"
	"path/filepath"

	"github.com/erros-gg/journal-companion/internal/config"
)

// DeriveSavedVarsDir finds the watch entry labelled "eso-savedvariables" and
// returns its parent directory — the ESO SavedVariables folder where
// JournalPrices.lua should be written.
//
// Returns an error if no such entry is configured. This is expected for users
// who haven't set up ESO upload watching yet; the Syncer refuses to start rather
// than writing to an unknown location.
func DeriveSavedVarsDir(watches []config.Watch) (string, error) {
	for _, w := range watches {
		if w.Label == "eso-savedvariables" {
			return filepath.Dir(w.Path), nil
		}
	}
	return "", errors.New("no eso-savedvariables watch configured")
}
