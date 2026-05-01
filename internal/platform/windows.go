//go:build windows

package platform

import (
	"errors"
	"os/exec"
	"syscall"
)

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

// IsFileLocked reports whether err is a Windows sharing-violation error,
// meaning the file is held open by another process.
func IsFileLocked(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == 0x20 // ERROR_SHARING_VIOLATION
}
