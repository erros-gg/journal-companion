package syncer

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// doSync performs one fetch-and-write cycle. Returns nil on success or 304.
func (s *Syncer) doSync(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.SnapshotURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if s.cfg.TokenFunc != nil {
		if token := s.cfg.TokenFunc(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	s.mu.Lock()
	if s.lastETag != "" {
		req.Header.Set("If-None-Match", s.lastETag)
	}
	if s.lastModified != "" {
		req.Header.Set("If-Modified-Since", s.lastModified)
	}
	s.mu.Unlock()

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, s.cfg.SnapshotURL)
	}

	contentType := resp.Header.Get("Content-Type")
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip reader (url=%s content-type=%q): %w", s.cfg.SnapshotURL, contentType, err)
	}
	defer gzr.Close()

	body, err := io.ReadAll(gzr)
	if err != nil {
		return fmt.Errorf("read decompressed body: %w", err)
	}

	target := filepath.Join(s.cfg.SavedVarsDir, "JournalPrices.lua")
	if err := atomicWrite(target, body); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}

	s.mu.Lock()
	s.lastETag = resp.Header.Get("ETag")
	s.lastModified = resp.Header.Get("Last-Modified")
	s.mu.Unlock()

	return nil
}

// atomicWrite writes content to targetPath via a temp file in the same
// directory, then renames. On Windows, os.Rename within the same directory
// is atomic from ESO's perspective — a reader either sees the old file or the
// new one, never a partial write.
func atomicWrite(targetPath string, content []byte) error {
	dir := filepath.Dir(targetPath)

	tmp, err := os.CreateTemp(dir, "JournalPrices.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after a successful rename

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
