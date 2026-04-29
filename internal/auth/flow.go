// Package auth implements the device-token browser flow (Phase 2).
//
// Phase 2 flow:
//  1. Companion generates a session ID and starts a localhost HTTP server.
//  2. Opens the browser to /companion/authorize?session=<id>.
//  3. User signs in (Supabase) and clicks "Authorize" with an editable device label.
//  4. Server stores a device_tokens row and redirects to the localhost callback URL.
//  5. Companion receives the token via the callback, stores it in the OS keychain,
//     shuts down the local server, and updates the tray.
package auth
