//go:build darwin

package platform

import "os/exec"

func AppName() string { return "Journal Companion" }

func OpenFolder(path string) error {
	return exec.Command("open", path).Start()
}

// SetStartWithOS writes or removes a LaunchAgent plist.
// TODO Phase 5: implement macOS launch agent.
func SetStartWithOS(exePath string, enable bool) error {
	return nil
}
