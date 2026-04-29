// Package uploader handles file compression, HTTP upload to the Journal API,
// result polling, and local SQLite recording.
//
// Upload protocol:
//
//	POST /api/companion/upload
//	  Authorization: Bearer <device-token>
//	  Content-Type: application/octet-stream
//	  Content-Encoding: gzip
//	  X-Companion-Label: <label>
//	  X-Companion-Filename: <basename>
//	  X-Companion-Filesize-Original: <bytes>
//	  X-Companion-Filehash-Original: sha256:<hex>
//	  X-Companion-Companion-Version: <version>
//	  X-Companion-OS: <os>
//	  body: gzip-compressed raw file bytes
//
//	202 response: {"uploadId":"…","statusUrl":"/api/companion/upload/…","queuedAt":"…"}
//
// Status polling:
//
//	GET /api/companion/upload/:id
//	  2s interval for first 30s, 10s interval thereafter, 5min total timeout
package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/erros-gg/journal-companion/internal/db"
)

// UploadResult summarises a completed upload for tray display and DB storage.
type UploadResult struct {
	UploadID    string
	Status      string // "completed" | "failed" | "polling_timeout" | "network_error"
	Summary     string // human-readable for the "Last upload" tray line
	Accepted    *int
	Rejected    *int
	Duplicate   *int
	Warnings    *string // JSON array
	ErrorCode   *string
	UserMessage *string
}

// Client wraps an HTTP client configured for a specific Journal API base URL.
type Client struct {
	apiBase    string
	version    string
	httpClient *http.Client
}

// New returns a Client targeting apiBase with the given companion version string.
func New(apiBase, version string) *Client {
	return &Client{
		apiBase: apiBase,
		version: version,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Upload compresses path, POSTs to /api/companion/upload, records the attempt in
// database, polls until a terminal status, updates the database row, and returns
// a summary result. A network error causes the upload row to remain in "pending"
// status for later retry.
func (c *Client) Upload(token, label, path string, database *db.DB) (UploadResult, error) {
	fi, err := CompressFile(path)
	if err != nil {
		return UploadResult{}, fmt.Errorf("compress %s: %w", path, err)
	}

	now := time.Now().UTC()
	uploadResult, err := c.post(token, label, path, fi)
	if err != nil {
		// Network or 4xx error — queue locally so the user can see something happened.
		slog.Error("upload POST failed", "path", path, "err", err)
		_ = database.RecordUpload(db.Upload{
			ID:       fmt.Sprintf("pending_%d", now.UnixNano()),
			Label:    label,
			FilePath: path,
			FileHash: fi.Hash,
			FileSize: fi.OriginalSize,
			Status:   "pending",
			QueuedAt: now,
		})
		return UploadResult{
			Status:  "network_error",
			Summary: "Upload pending — check connection",
		}, err
	}

	// Record the upload immediately with status "pending".
	queuedAt, _ := time.Parse(time.RFC3339, uploadResult.QueuedAt)
	if queuedAt.IsZero() {
		queuedAt = now
	}
	_ = database.RecordUpload(db.Upload{
		ID:       uploadResult.UploadID,
		Label:    label,
		FilePath: path,
		FileHash: fi.Hash,
		FileSize: fi.OriginalSize,
		Status:   "pending",
		QueuedAt: queuedAt,
	})

	// Poll for the parse result.
	result, err := c.poll(token, uploadResult.UploadID)
	if err != nil {
		slog.Error("upload poll failed", "uploadId", uploadResult.UploadID, "err", err)
		result = UploadResult{
			UploadID: uploadResult.UploadID,
			Status:   "polling_timeout",
			Summary:  "Uploaded — parse result pending",
		}
	}

	// Write result back to DB.
	completedAt := time.Now().UTC()
	_ = database.UpdateUploadResult(result.UploadID, result.Status, completedAt, db.Upload{
		ObservationsAccepted:  result.Accepted,
		ObservationsRejected:  result.Rejected,
		ObservationsDuplicate: result.Duplicate,
		Warnings:              result.Warnings,
		ErrorCode:             result.ErrorCode,
		ErrorUserMessage:      result.UserMessage,
	})

	return result, nil
}

// postResponse is the 202 body from POST /api/companion/upload.
type postResponse struct {
	UploadID  string `json:"uploadId"`
	StatusURL string `json:"statusUrl"`
	QueuedAt  string `json:"queuedAt"`
}

// post sends the compressed file and returns the server's 202 response.
func (c *Client) post(token, label, path string, fi FileInfo) (postResponse, error) {
	req, err := http.NewRequest(http.MethodPost, c.apiBase+"/api/companion/upload",
		bytes.NewReader(fi.Compressed))
	if err != nil {
		return postResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-Companion-Label", label)
	req.Header.Set("X-Companion-Filename", filepath.Base(path))
	req.Header.Set("X-Companion-Filesize-Original", strconv.FormatInt(fi.OriginalSize, 10))
	req.Header.Set("X-Companion-Filehash-Original", fi.Hash)
	req.Header.Set("X-Companion-Companion-Version", c.version)
	req.Header.Set("X-Companion-OS", runtime.GOOS)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return postResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return postResponse{}, errors.New("token rejected — sign in again")
	}
	if resp.StatusCode != http.StatusAccepted {
		return postResponse{}, fmt.Errorf("upload returned %d", resp.StatusCode)
	}

	var pr postResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return postResponse{}, err
	}
	return pr, nil
}

// statusResponse represents GET /api/companion/upload/:id response.
type statusResponse struct {
	UploadID    string `json:"uploadId"`
	Status      string `json:"status"`
	CompletedAt string `json:"completedAt"`
	Result      *struct {
		ObservationsAccepted  int      `json:"observationsAccepted"`
		ObservationsRejected  int      `json:"observationsRejected"`
		ObservationsDuplicate int      `json:"observationsDuplicate"`
		Warnings              []string `json:"warnings"`
	} `json:"result"`
	Error *struct {
		Code        string `json:"code"`
		UserMessage string `json:"userMessage"`
		Message     string `json:"message"`
	} `json:"error"`
}

// poll queries the upload status URL with the backoff schedule from the spec:
// every 2s for the first 30s, then every 10s up to 5 minutes total.
func (c *Client) poll(token, uploadID string) (UploadResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	url := c.apiBase + "/api/companion/upload/" + uploadID
	start := time.Now()

	for {
		interval := 2 * time.Second
		if time.Since(start) > 30*time.Second {
			interval = 10 * time.Second
		}

		select {
		case <-ctx.Done():
			return UploadResult{}, errors.New("polling timed out")
		case <-time.After(interval):
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return UploadResult{}, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			slog.Warn("poll request failed", "uploadId", uploadID, "err", err)
			continue
		}

		var sr statusResponse
		_ = json.NewDecoder(resp.Body).Decode(&sr)
		resp.Body.Close()

		switch sr.Status {
		case "completed":
			return completedResult(uploadID, sr), nil
		case "failed":
			return failedResult(uploadID, sr), nil
		}
		// "queued" or "parsing" — keep polling
	}
}

func completedResult(uploadID string, sr statusResponse) UploadResult {
	r := UploadResult{
		UploadID: uploadID,
		Status:   "completed",
	}
	if sr.Result != nil {
		a, rj, d := sr.Result.ObservationsAccepted, sr.Result.ObservationsRejected, sr.Result.ObservationsDuplicate
		r.Accepted = &a
		r.Rejected = &rj
		r.Duplicate = &d
		r.Summary = fmt.Sprintf("%d accepted, %d duplicate", a, d)
		if len(sr.Result.Warnings) > 0 {
			b, _ := json.Marshal(sr.Result.Warnings)
			s := string(b)
			r.Warnings = &s
		}
	}
	if r.Summary == "" {
		r.Summary = "Upload completed"
	}
	return r
}

func failedResult(uploadID string, sr statusResponse) UploadResult {
	r := UploadResult{UploadID: uploadID, Status: "failed"}
	if sr.Error != nil {
		r.ErrorCode = &sr.Error.Code
		r.UserMessage = &sr.Error.UserMessage
		r.Summary = sr.Error.UserMessage
		slog.Error("parse failed", "uploadId", uploadID, "code", sr.Error.Code, "detail", sr.Error.Message)
	}
	if r.Summary == "" {
		r.Summary = "Upload failed"
	}
	return r
}
