// Package auth implements the device-token browser flow.
//
// Contract with the server (endpoints not yet deployed; built in parallel):
//
//	POST /api/companion/auth/initiate
//	  body:     {"sessionId":"…","callbackPort":53219,"deviceLabel":"…"}
//	  response: {"authUrl":"https://journal.erros.gg/companion/authorize?session=…"}
//
//	POST /api/companion/auth/poll
//	  body:     {"sessionId":"…"}
//	  response: {"status":"pending"}
//	            {"status":"authorized","token":"…","username":"…"}
//	            {"status":"expired"}
//	            {"status":"error","error":"…"}
//
//	Localhost callback (browser redirect):
//	  GET http://localhost:{callbackPort}/callback?token=…&username=…
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/erros-gg/journal-companion/internal/platform"
)

const (
	preferredPort = 53219
	pollInterval  = 2 * time.Second
	flowTimeout   = 5 * time.Minute
)

// Result is returned by StartFlow on success.
type Result struct {
	Token    string
	Username string
}

// StartFlow runs the browser-based device-token auth flow and blocks until
// it completes (success, user cancellation, or timeout). On success the token
// and username are persisted in the OS keychain before returning.
//
// apiBase is the value from config.Network.APIBase (e.g. "https://journal.erros.gg").
// deviceLabel is the human-readable name shown on the Journal authorise page.
func StartFlow(apiBase, deviceLabel string) (Result, error) {
	sessionID, err := randomHex(16)
	if err != nil {
		return Result{}, fmt.Errorf("generate session id: %w", err)
	}

	// Start the localhost callback listener.
	listener, port, err := listenOnPort(preferredPort)
	if err != nil {
		return Result{}, fmt.Errorf("start callback listener: %w", err)
	}

	// tokenCh receives the token from whichever path arrives first:
	// the localhost redirect or the poll loop.
	tokenCh := make(chan Result, 1)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), flowTimeout)
	defer cancel()

	// Serve a single /callback request, then stop.
	srv := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		username := r.URL.Query().Get("username")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "<html><body><p>Authorised. You can close this tab.</p></body></html>")
		select {
		case tokenCh <- Result{Token: token, Username: username}:
		default:
		}
		// Shut down after the response is sent.
		go srv.Shutdown(context.Background()) //nolint:errcheck
	})
	srv.Handler = mux

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("auth callback server", "err", err)
		}
	}()
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	// Call /api/companion/auth/initiate to get the browser URL.
	authURL, err := initiate(ctx, apiBase, sessionID, port, deviceLabel)
	if err != nil {
		return Result{}, fmt.Errorf("initiate auth: %w", err)
	}
	slog.Info("auth flow initiated", "sessionID", sessionID, "port", port)

	if err := platform.OpenBrowser(authURL); err != nil {
		slog.Warn("could not open browser automatically", "url", authURL, "err", err)
	}

	// Poll in parallel with the callback listener.
	go func() {
		result, err := pollLoop(ctx, apiBase, sessionID)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		select {
		case tokenCh <- result:
		default:
		}
	}()

	// Wait for first result or timeout.
	select {
	case result := <-tokenCh:
		if err := SaveToken(result.Token); err != nil {
			return Result{}, fmt.Errorf("save token: %w", err)
		}
		if result.Username != "" {
			_ = SaveUsername(result.Username)
		}
		slog.Info("auth flow complete", "username", result.Username)
		return result, nil

	case err := <-errCh:
		return Result{}, err

	case <-ctx.Done():
		return Result{}, errors.New("auth flow timed out after 5 minutes")
	}
}

// initiate calls POST /api/companion/auth/initiate and returns the browser URL.
func initiate(ctx context.Context, apiBase, sessionID string, port int, deviceLabel string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"sessionId":    sessionID,
		"callbackPort": port,
		"deviceLabel":  deviceLabel,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/api/companion/auth/initiate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("initiate returned %d", resp.StatusCode)
	}

	var payload struct {
		AuthURL string `json:"authUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AuthURL == "" {
		return "", errors.New("server returned empty authUrl")
	}
	return payload.AuthURL, nil
}

// pollLoop calls POST /api/companion/auth/poll every 2 seconds until it gets
// an "authorized" status or the context is cancelled.
func pollLoop(ctx context.Context, apiBase, sessionID string) (Result, error) {
	body, _ := json.Marshal(map[string]string{"sessionId": sessionID})

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return Result{}, errors.New("poll loop cancelled")
		case <-ticker.C:
		}

		var pollResp struct {
			Status   string `json:"status"`
			Token    string `json:"token"`
			Username string `json:"username"`
			Error    string `json:"error"`
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			apiBase+"/api/companion/auth/poll", bytes.NewReader(body))
		if err != nil {
			return Result{}, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Warn("auth poll request failed", "err", err)
			continue // transient error — keep polling
		}
		_ = json.NewDecoder(resp.Body).Decode(&pollResp)
		resp.Body.Close()

		switch pollResp.Status {
		case "authorized":
			return Result{Token: pollResp.Token, Username: pollResp.Username}, nil
		case "expired":
			return Result{}, errors.New("auth session expired")
		case "error":
			return Result{}, fmt.Errorf("auth error: %s", pollResp.Error)
		}
		// "pending" or anything else: continue polling
	}
}

// listenOnPort tries port first, then lets the OS pick.
func listenOnPort(port int) (net.Listener, int, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		// Port in use — let OS assign one.
		l, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, 0, err
		}
	}
	return l, l.Addr().(*net.TCPAddr).Port, nil
}

// randomHex returns n random bytes encoded as a hex string.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
