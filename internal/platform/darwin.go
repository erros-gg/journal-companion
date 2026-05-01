//go:build darwin

package platform

import "os/exec"

func AppName() string { return "Journal Companion" }

func OSName() string { return "darwin" }

func OpenFolder(path string) error {
	return exec.Command("open", path).Start()
}

// OpenBrowser opens url in the default macOS browser.
func OpenBrowser(url string) error {
	return exec.Command("open", url).Start()
}

// SetStartWithOS writes or removes a LaunchAgent plist.
// TODO Phase 5: implement macOS launch agent.
func SetStartWithOS(exePath string, enable bool) error {
	return ErrNotImplemented
}

// IsFileLocked reports whether the error indicates an exclusive file lock.
// On macOS this is not a common problem; file locking semantics differ from Windows.
func IsFileLocked(err error) bool { return false }
