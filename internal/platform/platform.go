// Package platform abstracts OS-specific behaviour behind a uniform interface.
// Each function's implementation lives in a build-tagged file:
// windows.go, darwin.go, linux.go.
package platform

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotImplemented is returned by platform stubs pending a future phase.
var ErrNotImplemented = errors.New("not yet implemented")

// ConfigDir returns the platform-appropriate directory for all app data:
// config file, database, and log file.
//
// Windows: %APPDATA%\Journal Companion
// macOS:   ~/Library/Application Support/Journal Companion
// Linux:   ~/.config/journal-companion
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName()), nil
}
