//go:build windows

package platform

import "os/exec"

func AppName() string { return "Journal Companion" }

// OSName returns the OS identifier sent in the X-Companion-OS upload header.
func OSName() string { return "windows" }

// OpenFolder opens path in Windows Explorer.
func OpenFolder(path string) error {
	return exec.Command("explorer.exe", path).Start()
}

// OpenBrowser opens url in the default Windows browser.
func OpenBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// SetStartWithOS writes or removes the app from the Windows startup registry key.
// TODO Phase 4: implement using golang.org/x/sys/windows/registry.
func SetStartWithOS(exePath string, enable bool) error {
	return ErrNotImplemented
}
