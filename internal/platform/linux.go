//go:build linux

package platform

import "os/exec"

func AppName() string { return "journal-companion" }

func OSName() string { return "linux" }

func OpenFolder(path string) error {
	return exec.Command("xdg-open", path).Start()
}

// OpenBrowser opens url in the default Linux browser.
func OpenBrowser(url string) error {
	return exec.Command("xdg-open", url).Start()
}

// SetStartWithOS writes or removes an XDG autostart .desktop file.
// TODO Phase 5: implement XDG autostart.
func SetStartWithOS(exePath string, enable bool) error {
	return ErrNotImplemented
}
