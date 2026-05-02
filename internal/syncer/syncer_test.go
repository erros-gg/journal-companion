package syncer

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/erros-gg/journal-companion/internal/config"
)

// noopReporter satisfies StatusReporter for tests that don't need status checks.
type noopReporter struct{}

func (noopReporter) UpdatePriceSyncStatus(SyncStatus) {}

// capturingReporter records status updates for assertion.
type capturingReporter struct {
	statuses []SyncStatus
}

func (r *capturingReporter) UpdatePriceSyncStatus(s SyncStatus) {
	r.statuses = append(r.statuses, s)
}

func gzipBytes(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ── Path resolution ──────────────────────────────────────────────────────────

func TestDeriveSavedVarsDir_Found(t *testing.T) {
	const watchPath = `C:\Users\test\SavedVariables\Journal.lua`
	watches := []config.Watch{
		{Label: "other", Path: `C:\Users\test\other.lua`},
		{Label: "eso-savedvariables", Path: watchPath},
	}
	dir, err := DeriveSavedVarsDir(watches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Dir(watchPath)
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestDeriveSavedVarsDir_Missing(t *testing.T) {
	_, err := DeriveSavedVarsDir(nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	_, err = DeriveSavedVarsDir([]config.Watch{{Label: "other", Path: "/x/y.lua"}})
	if err == nil {
		t.Fatal("expected error when label absent, got nil")
	}
}

func TestDeriveAddonDir_Found(t *testing.T) {
	const watchPath = `C:\Users\test\SavedVariables\Journal.lua`
	watches := []config.Watch{
		{Label: "eso-savedvariables", Path: watchPath},
	}
	dir, err := DeriveAddonDir(watches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(filepath.Dir(filepath.Dir(watchPath)), "AddOns", "Journal")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestDeriveAddonDir_Missing(t *testing.T) {
	_, err := DeriveAddonDir(nil)
	if err == nil {
		t.Fatal("expected error when no watches configured")
	}
}

// ── Atomic write ─────────────────────────────────────────────────────────────

func TestAtomicWrite_WritesContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "JournalPrices.lua")
	want := []byte("JournalPrices = {}")

	if err := atomicWrite(target, want); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("content mismatch: got %q, want %q", got, want)
	}
}

func TestAtomicWrite_NoTempFilesLeft(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "JournalPrices.lua")

	if err := atomicWrite(target, []byte("x")); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "JournalPrices.lua" {
			t.Errorf("unexpected file left in dir: %s", e.Name())
		}
	}
}

func TestAtomicWrite_Overwrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "JournalPrices.lua")
	os.WriteFile(target, []byte("old"), 0600)

	if err := atomicWrite(target, []byte("new")); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}

// ── Conditional GET ──────────────────────────────────────────────────────────

func TestDoSync_200WritesFile(t *testing.T) {
	const content = `JournalPrices = { generatedAt = 1, prices = {} }`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"etag1"`)
		w.WriteHeader(http.StatusOK)
		w.Write(gzipBytes(t, content))
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := New(Config{Interval: time.Hour, SnapshotURL: srv.URL, AddonDir: dir}, true, noopReporter{})

	if err := s.doSync(context.Background()); err != nil {
		t.Fatalf("doSync: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "JournalPrices.lua"))
	if err != nil {
		t.Fatal("JournalPrices.lua not written:", err)
	}
	if string(got) != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestDoSync_304SkipsWrite(t *testing.T) {
	const etag = `"etag1"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)
		w.Write(gzipBytes(t, "original"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := New(Config{Interval: time.Hour, SnapshotURL: srv.URL, AddonDir: dir}, true, noopReporter{})

	// First sync: 200, file written.
	if err := s.doSync(context.Background()); err != nil {
		t.Fatalf("first doSync: %v", err)
	}

	// Overwrite with sentinel so we can detect if a second write happens.
	sentinel := []byte("sentinel")
	os.WriteFile(filepath.Join(dir, "JournalPrices.lua"), sentinel, 0600)

	// Second sync: server returns 304 → file must not be rewritten.
	if err := s.doSync(context.Background()); err != nil {
		t.Fatalf("second doSync: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "JournalPrices.lua"))
	if !bytes.Equal(got, sentinel) {
		t.Errorf("file was rewritten on 304: got %q, want sentinel", got)
	}
}

func TestDoSync_CachesConditionalHeaders(t *testing.T) {
	var receivedIfNoneMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"myetag"`)
		w.WriteHeader(http.StatusOK)
		w.Write(gzipBytes(t, "data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := New(Config{Interval: time.Hour, SnapshotURL: srv.URL, AddonDir: dir}, true, noopReporter{})

	// First request: no conditional headers sent yet.
	s.doSync(context.Background())
	if receivedIfNoneMatch != "" {
		t.Errorf("first request should send no If-None-Match, got %q", receivedIfNoneMatch)
	}

	// Second request: ETag from first response should be forwarded.
	s.doSync(context.Background())
	if receivedIfNoneMatch != `"myetag"` {
		t.Errorf("second request: got If-None-Match=%q, want %q", receivedIfNoneMatch, `"myetag"`)
	}
}

// ── Error handling ───────────────────────────────────────────────────────────

func TestDoSync_HTTPErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := New(Config{Interval: time.Hour, SnapshotURL: srv.URL, AddonDir: dir}, true, noopReporter{})

	if err := s.doSync(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "JournalPrices.lua")); !os.IsNotExist(err) {
		t.Error("file should not exist after HTTP error")
	}
}

func TestSyncOnce_ReportsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	rep := &capturingReporter{}
	s := New(Config{Interval: time.Hour, SnapshotURL: srv.URL, AddonDir: dir}, true, rep)

	s.syncOnce(context.Background())

	if len(rep.statuses) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(rep.statuses))
	}
	if rep.statuses[0].LastError == nil {
		t.Error("expected non-nil LastError after 500 response")
	}
	if rep.statuses[0].LastSyncAt.IsZero() {
		t.Error("expected non-zero LastSyncAt")
	}
}

func TestSyncOnce_SuccessReportsNilError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"e1"`)
		w.WriteHeader(http.StatusOK)
		w.Write(gzipBytes(t, "data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	rep := &capturingReporter{}
	s := New(Config{Interval: time.Hour, SnapshotURL: srv.URL, AddonDir: dir}, true, rep)

	s.syncOnce(context.Background())

	if len(rep.statuses) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(rep.statuses))
	}
	if rep.statuses[0].LastError != nil {
		t.Errorf("expected nil LastError, got: %v", rep.statuses[0].LastError)
	}
}
