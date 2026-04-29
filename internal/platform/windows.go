//go:build windows

package platform

import "os/exec"

// AppName returns the Windows-conventional app folder name (with spaces, title case).
func AppName() string { return "Journal Companion" }

// OpenFolder opens path in Windows Explorer.
func OpenFolder(path string) error {
	return exec.Command("explorer.exe", path).Start()
}

// SetStartWithOS writes or removes the app from the Windows startup registry key:
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run
// TODO Phase 4: implement using golang.org/x/sys/windows/registry.
func SetStartWithOS(exePath string, enable bool) error {
	return nil
}
