//go:build linux

package platform

import "os/exec"

// AppName returns a lowercase-no-spaces name, following XDG conventions.
func AppName() string { return "journal-companion" }

func OpenFolder(path string) error {
	return exec.Command("xdg-open", path).Start()
}

// SetStartWithOS writes or removes an XDG autostart .desktop file.
// TODO Phase 5: implement XDG autostart.
func SetStartWithOS(exePath string, enable bool) error {
	return nil
}
