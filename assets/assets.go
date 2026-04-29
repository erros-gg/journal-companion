package assets

import _ "embed"

// Icon is the system tray icon.
// Replace assets/icon.ico with the brand asset (32×32 recommended) before shipping.
//
//go:embed icon.ico
var Icon []byte
