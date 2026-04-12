package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/briansurratt/annotate/internal/index"
	"github.com/briansurratt/annotate/internal/queue"
	"github.com/briansurratt/annotate/internal/store"
)

// openTestDB opens an in-memory SQLite database with full schema migrations.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(context.Background(), filepath.Join(dir, "db.sqlite"), nil)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s.DB()
}

// newTestQueue creates a Queue backed by db.
func newTestQueue(t *testing.T, db *sql.DB) *queue.Queue {
	t.Helper()
	q, err := queue.New(db)
	if err != nil {
		t.Fatalf("queue.New: %v", err)
	}
	t.Cleanup(q.Close)
	return q
}

// shortSocketPath returns a Unix socket path short enough to satisfy macOS's
// 104-byte sun_path limit. It uses os.TempDir() which is typically /tmp.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	// Use a short prefix + test name hash to stay under 104 bytes.
	dir, err := os.MkdirTemp("", "ann-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}

// defaultConfig returns a minimal valid Config for testing, using a short
// debounce and flush interval so tests don't need to sleep long.
func defaultConfig(t *testing.T) Config {
	t.Helper()
	db := openTestDB(t)
	return Config{
		DB:               db,
		Index:            index.New(),
		Queue:            newTestQueue(t, db),
		WorkspaceRoot:    t.TempDir(),
		SocketPath:       shortSocketPath(t),
		DebounceInterval: 5 * time.Millisecond,
		FlushInterval:    50 * time.Millisecond,
	}
}

// startDaemon starts d.Run in a goroutine and returns a cancel func plus a
// done channel that is closed when Run returns. Receiving from a closed channel
// is always safe, so the channel can be used in select statements multiple times.
func startDaemon(t *testing.T, d *Daemon) (cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan struct{})
	go func() {
		d.Run(ctx) //nolint:errcheck
		close(ch)
	}()
	t.Cleanup(func() {
		cancel()
		<-ch
	})
	return cancel, ch
}

// ─── Section 1: /enqueue HTTP handler ───────────────────────────────────────

func TestEnqueueEndpoint_Returns200AndEnqueuesPath(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	// Wait for socket to appear.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(cfg.SocketPath); statErr == nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", cfg.SocketPath)
			},
		},
	}

	filePath := "/notes/enqueued-via-http.md"
	resp, err := client.Post("http://unix/enqueue", "text/plain", strings.NewReader(filePath))
	if err != nil {
		t.Fatalf("POST /enqueue: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /enqueue status = %d, want 200", resp.StatusCode)
	}

	// Verify an 'enqueued' event was logged for the path.
	time.Sleep(50 * time.Millisecond)
	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM event_log WHERE event_type = 'enqueued' AND file_path = ?`, filePath,
	).Scan(&count); err != nil {
		t.Fatalf("query event_log: %v", err)
	}
	if count == 0 {
		t.Errorf("no 'enqueued' event_log entry for %s after POST /enqueue", filePath)
	}
}

// ─── Section 2: daemon.New ──────────────────────────────────────────────────

func TestNew_ReturnsErrorOnNilDB(t *testing.T) {
	db := openTestDB(t)
	q := newTestQueue(t, db)
	cfg := Config{
		DB:               nil,
		Index:            index.New(),
		Queue:            q,
		WorkspaceRoot:    t.TempDir(),
		SocketPath:       filepath.Join(t.TempDir(), "d.sock"),
		DebounceInterval: time.Millisecond,
		FlushInterval:    time.Millisecond,
	}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("New() with nil DB: want error, got nil")
	}
}

func TestNew_SucceedsWithFullConfig(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with full config: %v", err)
	}
	if d == nil {
		t.Fatal("New() returned nil Daemon")
	}
}

// ─── Section 3: httpServer ───────────────────────────────────────────────────

func TestRun_SocketExistsWithin100ms(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Errorf("socket file not created at %s within 100ms", cfg.SocketPath)
}

func TestRun_StatusEndpointReturns200(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	// Wait for socket.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(cfg.SocketPath); statErr == nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", cfg.SocketPath)
			},
		},
	}

	resp, err := client.Get("http://unix/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /status status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /status body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
}

func TestRun_SocketRemovedAfterCancel(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, done := startDaemon(t, d)

	// Wait for socket to appear.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(cfg.SocketPath); statErr == nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after cancel")
	}

	if _, err := os.Stat(cfg.SocketPath); !os.IsNotExist(err) {
		t.Errorf("socket file %s still exists after shutdown", cfg.SocketPath)
	}
}

// ─── Section 4: fsWatcher ───────────────────────────────────────────────────

func TestFsWatcher_SingleSaveEnqueues(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.DebounceInterval = 10 * time.Millisecond
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	// Give watcher time to start.
	time.Sleep(50 * time.Millisecond)

	mdPath := filepath.Join(cfg.WorkspaceRoot, "note.md")
	if err := os.WriteFile(mdPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Wait for debounce to fire + enqueue. Check event_log rather than the queue
	// directly because the queueWorker may dequeue the entry before the test can.
	time.Sleep(100 * time.Millisecond)

	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM event_log WHERE event_type = 'enqueued' AND file_path = ?`, mdPath,
	).Scan(&count); err != nil {
		t.Fatalf("query event_log: %v", err)
	}
	if count == 0 {
		t.Fatalf("no 'enqueued' event_log entry for %s", mdPath)
	}
}

func TestFsWatcher_RapidSavesDeduplicate(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.DebounceInterval = 30 * time.Millisecond
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	time.Sleep(50 * time.Millisecond)

	mdPath := filepath.Join(cfg.WorkspaceRoot, "note.md")
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(mdPath, []byte(fmt.Sprintf("v%d", i)), 0o644); err != nil {
			t.Fatalf("write file iter %d: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Wait well past the debounce interval for enqueue to happen.
	time.Sleep(150 * time.Millisecond)

	// Three rapid writes should produce exactly one 'enqueued' event (debounce dedup).
	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM event_log WHERE event_type = 'enqueued' AND file_path = ?`, mdPath,
	).Scan(&count); err != nil {
		t.Fatalf("query event_log: %v", err)
	}
	if count == 0 {
		t.Fatal("expected one 'enqueued' event, got 0")
	}
	if count > 1 {
		t.Errorf("rapid saves deduplicated: expected 1 'enqueued' event, got %d", count)
	}
}

func TestFsWatcher_NonMarkdownIgnored(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.DebounceInterval = 10 * time.Millisecond
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	time.Sleep(50 * time.Millisecond)

	txtPath := filepath.Join(cfg.WorkspaceRoot, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	goPath := filepath.Join(cfg.WorkspaceRoot, "main.go")
	if err := os.WriteFile(goPath, []byte("package main"), 0o644); err != nil {
		t.Fatalf("write go: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	entry, err := cfg.Queue.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if entry != nil {
		t.Errorf("expected empty queue for non-.md files, got entry: %+v", entry)
	}
}

func TestFsWatcher_TwoPathsEnqueueIndependently(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.DebounceInterval = 20 * time.Millisecond
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	time.Sleep(50 * time.Millisecond)

	pathA := filepath.Join(cfg.WorkspaceRoot, "a.md")
	pathB := filepath.Join(cfg.WorkspaceRoot, "b.md")

	if err := os.WriteFile(pathA, []byte("a"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(pathB, []byte("b"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	// Wait for both debounce timers to fire. Check event_log because the
	// queueWorker may dequeue entries before we can inspect the queue.
	time.Sleep(150 * time.Millisecond)

	check := func(path string) {
		var count int
		if err := cfg.DB.QueryRow(
			`SELECT COUNT(*) FROM event_log WHERE event_type = 'enqueued' AND file_path = ?`, path,
		).Scan(&count); err != nil {
			t.Fatalf("query event_log for %s: %v", path, err)
		}
		if count == 0 {
			t.Errorf("%s was not enqueued", path)
		}
	}
	check(pathA)
	check(pathB)
}

// ─── Section 5: queueWorker ─────────────────────────────────────────────────

func TestQueueWorker_DequeuesAndLogsProcessed(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-populate the queue.
	if err := cfg.Queue.Enqueue("/notes/foo.md", time.Now().UnixMilli()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	// Wait for worker to process.
	time.Sleep(300 * time.Millisecond)

	// Queue should now be empty (worker removed the entry).
	entry, err := cfg.Queue.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue after worker: %v", err)
	}
	if entry != nil {
		t.Errorf("queue not empty after worker processed entry: %+v", entry)
	}

	// Verify a processed event was logged.
	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM event_log WHERE event_type = 'processed' AND file_path = '/notes/foo.md'`,
	).Scan(&count); err != nil {
		t.Fatalf("query event_log: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one 'processed' event_log entry, got 0")
	}
}

func TestQueueWorker_EmptyQueueProducesNoEvents(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	time.Sleep(250 * time.Millisecond)

	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM event_log WHERE event_type = 'processed'`,
	).Scan(&count); err != nil {
		t.Fatalf("query event_log: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 processed events for empty queue, got %d", count)
	}
}

func TestQueueWorker_ExitsOnContextCancel(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, done := startDaemon(t, d)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s after cancel")
	}
}

// ─── Section 6: indexFlusher ────────────────────────────────────────────────

func TestIndexFlusher_FlushDirtyCalledOnInterval(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.FlushInterval = 50 * time.Millisecond

	// Put a dirty entry in the index so we can detect a flush.
	cfg.Index.Update(&index.NoteEntry{
		Path:  "/notes/test.md",
		Title: "Test",
		Hash:  "abc",
	})

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	// Wait 3× the flush interval.
	time.Sleep(150 * time.Millisecond)

	// Verify the entry was flushed to SQLite.
	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM index_cache WHERE file_path = '/notes/test.md'`,
	).Scan(&count); err != nil {
		t.Fatalf("query index_cache: %v", err)
	}
	if count == 0 {
		t.Error("FlushDirty not called within 3× flush interval")
	}
}

func TestIndexFlusher_FlushCalledOnShutdown(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.FlushInterval = 10 * time.Second // Long interval so only shutdown flush triggers.

	cfg.Index.Update(&index.NoteEntry{
		Path:  "/notes/shutdown.md",
		Title: "Shutdown",
		Hash:  "xyz",
	})

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, done := startDaemon(t, d)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after cancel")
	}

	// The shutdown flush (in indexFlusher or final flush in Run) should have
	// written the dirty entry.
	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM index_cache WHERE file_path = '/notes/shutdown.md'`,
	).Scan(&count); err != nil {
		t.Fatalf("query index_cache: %v", err)
	}
	if count == 0 {
		t.Error("FlushDirty not called on shutdown")
	}
}

// ─── Section 7: Daemon.Run and graceful shutdown ─────────────────────────────

func TestRun_AllGoroutinesStartWithin100ms(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(cfg.SocketPath); statErr == nil {
			return // httpServer is up, all goroutines started
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Error("httpServer did not start within 100ms (proxy for all goroutines starting)")
}

func TestRun_ContextCancelReturnsNilWithin1s(t *testing.T) {
	cfg := defaultConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan error, 1)
	go func() { ch <- d.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case runErr := <-ch:
		if runErr != nil {
			t.Errorf("Run returned %v after context cancel, want nil", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s after cancel")
	}
}

func TestRun_GoroutineErrorPropagates(t *testing.T) {
	cfg := defaultConfig(t)
	// Use an invalid socket path to force httpServer to fail immediately.
	cfg.SocketPath = "/nonexistent-dir/that-cannot-be-created/daemon.sock"
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan error, 1)
	go func() { ch <- d.Run(ctx) }()

	select {
	case runErr := <-ch:
		if runErr == nil {
			t.Error("Run returned nil but expected an error from httpServer")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after goroutine error")
	}
}

func TestRun_FinalFlushCalledAfterGoroutinesStop(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.FlushInterval = 10 * time.Second // Suppress periodic flushes.

	// Dirty entry added AFTER daemon starts — simulates a late write.
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, done := startDaemon(t, d)

	time.Sleep(50 * time.Millisecond)

	// Add dirty entry just before shutdown.
	cfg.Index.Update(&index.NoteEntry{
		Path:  "/notes/final.md",
		Title: "Final",
		Hash:  "zzz",
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s")
	}

	// The post-Wait final flush in Run should have persisted this entry.
	var count int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM index_cache WHERE file_path = '/notes/final.md'`,
	).Scan(&count); err != nil {
		t.Fatalf("query index_cache: %v", err)
	}
	if count == 0 {
		t.Error("final FlushDirty not called after all goroutines stopped")
	}
}

// ─── Section 8: Integration test ────────────────────────────────────────────

func TestIntegration_SaveFileEnqueuesAndLogsEvent(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.DebounceInterval = 10 * time.Millisecond

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancel, _ := startDaemon(t, d)
	defer cancel()

	// Give daemon time to fully start.
	time.Sleep(100 * time.Millisecond)

	mdPath := filepath.Join(cfg.WorkspaceRoot, "integration.md")
	if err := os.WriteFile(mdPath, []byte("# Integration Test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Wait for debounce + worker to process.
	time.Sleep(300 * time.Millisecond)

	// Verify enqueued event exists in event_log.
	var enqueuedCount int
	if err := cfg.DB.QueryRow(
		`SELECT COUNT(*) FROM event_log WHERE event_type = 'enqueued' AND file_path = ?`, mdPath,
	).Scan(&enqueuedCount); err != nil {
		t.Fatalf("query enqueued events: %v", err)
	}
	if enqueuedCount == 0 {
		t.Error("no 'enqueued' event_log entry for integration.md")
	}
}
