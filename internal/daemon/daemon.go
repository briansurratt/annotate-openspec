// Package daemon wires the foundational subsystems (store, index, queue,
// eventlog, fsnotify) into a long-running process. It manages four goroutines
// under a shared errgroup context: httpServer, queueWorker, indexFlusher, and
// fsWatcher. Graceful shutdown is coordinated via context cancellation followed
// by a 10-second drain timeout and a final index flush.
package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"

	"github.com/briansurratt/annotate/internal/eventlog"
	"github.com/briansurratt/annotate/internal/index"
	"github.com/briansurratt/annotate/internal/queue"
)

// Config holds all dependencies and tunable parameters for the Daemon.
type Config struct {
	// DB is the SQLite database connection shared by all subsystems.
	DB *sql.DB

	// Index is the in-memory note index.
	Index *index.Index

	// Queue is the FIFO deduplicating processing queue.
	Queue *queue.Queue

	// WorkspaceRoot is the directory to watch for markdown file changes.
	WorkspaceRoot string

	// SocketPath is the path of the Unix domain socket for the HTTP control plane.
	SocketPath string

	// DebounceInterval is how long to wait after the last filesystem event for
	// a given path before enqueuing it. Prevents churn from rapid saves.
	DebounceInterval time.Duration

	// FlushInterval controls how often indexFlusher calls index.FlushDirty.
	FlushInterval time.Duration
}

// Daemon coordinates four goroutines that make up the annotation daemon runtime.
// Construct with New and start with Run.
type Daemon struct {
	cfg Config
}

// New validates cfg and returns a ready-to-start Daemon.
// No goroutines are started; call Run to begin processing.
func New(cfg Config) (*Daemon, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("daemon.New: DB must not be nil")
	}
	if cfg.Index == nil {
		return nil, fmt.Errorf("daemon.New: Index must not be nil")
	}
	if cfg.Queue == nil {
		return nil, fmt.Errorf("daemon.New: Queue must not be nil")
	}
	if cfg.WorkspaceRoot == "" {
		return nil, fmt.Errorf("daemon.New: WorkspaceRoot must not be empty")
	}
	if cfg.SocketPath == "" {
		return nil, fmt.Errorf("daemon.New: SocketPath must not be empty")
	}
	if cfg.DebounceInterval <= 0 {
		return nil, fmt.Errorf("daemon.New: DebounceInterval must be positive")
	}
	if cfg.FlushInterval <= 0 {
		return nil, fmt.Errorf("daemon.New: FlushInterval must be positive")
	}
	return &Daemon{cfg: cfg}, nil
}

// Run launches all four goroutines and blocks until they all exit.
// It returns nil if the context was cancelled (clean shutdown) or the first
// non-nil error returned by any goroutine. A 10-second drain timeout is applied
// to goroutine shutdown; after all goroutines exit, index.FlushDirty is called
// once to capture any remaining dirty entries.
func (d *Daemon) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error { return d.httpServer(gctx) })
	g.Go(func() error { return d.queueWorker(gctx) })
	g.Go(func() error { return d.indexFlusher(gctx) })
	g.Go(func() error { return d.fsWatcher(gctx) })

	// Wait for all goroutines with a hard 10-second drain timeout.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- g.Wait()
	}()

	var err error
	select {
	case err = <-waitDone:
	case <-time.After(10 * time.Second):
		err = fmt.Errorf("daemon shutdown timed out after 10 seconds")
	}

	// Final flush to capture dirty entries written after the flusher goroutine stopped.
	if flushErr := d.cfg.Index.FlushDirty(d.cfg.DB); flushErr != nil {
		if err == nil {
			err = fmt.Errorf("final index flush: %w", flushErr)
		}
	}

	// A cancelled context is a clean shutdown — return nil.
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// httpServer listens on the Unix domain socket and serves the HTTP control plane.
// It removes any stale socket file before binding and removes it again on exit.
func (d *Daemon) httpServer(ctx context.Context) error {
	// Remove stale socket file if present.
	_ = os.Remove(d.cfg.SocketPath)

	// Ensure socket directory exists.
	if err := os.MkdirAll(filepath.Dir(d.cfg.SocketPath), 0o755); err != nil {
		return fmt.Errorf("httpServer: mkdir socket dir: %w", err)
	}

	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("httpServer: listen unix %s: %w", d.cfg.SocketPath, err)
	}
	defer os.Remove(d.cfg.SocketPath)

	mux := http.NewServeMux()
	mux.HandleFunc("/status", handleStatus)

	srv := &http.Server{Handler: mux}

	// Shut down the HTTP server when ctx is done.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("httpServer: serve: %w", err)
	}
	return nil
}

// handleStatus responds to GET /status with {"status":"ok"}.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// queueWorker polls the queue and logs a processed event for each entry.
// It sleeps 100ms when the queue is empty to avoid busy-waiting.
func (d *Daemon) queueWorker(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		entry, err := d.cfg.Queue.Dequeue()
		if err != nil {
			return fmt.Errorf("queueWorker: dequeue: %w", err)
		}

		if entry == nil {
			// Queue is empty; sleep briefly before polling again.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		if err := eventlog.Append(d.cfg.DB, eventlog.EventProcessed, entry.FilePath, nil); err != nil {
			return fmt.Errorf("queueWorker: append processed event: %w", err)
		}

		if err := d.cfg.Queue.Remove(entry.ID); err != nil {
			return fmt.Errorf("queueWorker: remove: %w", err)
		}
	}
}

// indexFlusher calls index.FlushDirty on a timer and once more on shutdown.
func (d *Daemon) indexFlusher(ctx context.Context) error {
	ticker := time.NewTicker(d.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush before returning.
			if err := d.cfg.Index.FlushDirty(d.cfg.DB); err != nil {
				return fmt.Errorf("indexFlusher: final flush: %w", err)
			}
			return nil
		case <-ticker.C:
			if err := d.cfg.Index.FlushDirty(d.cfg.DB); err != nil {
				return fmt.Errorf("indexFlusher: flush: %w", err)
			}
		}
	}
}

// fsWatcher watches WorkspaceRoot for filesystem events and enqueues changed
// .md files after a per-path debounce interval.
func (d *Daemon) fsWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsWatcher: new watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(d.cfg.WorkspaceRoot); err != nil {
		return fmt.Errorf("fsWatcher: watch %s: %w", d.cfg.WorkspaceRoot, err)
	}

	var (
		mu     sync.Mutex
		timers = make(map[string]*time.Timer)
	)

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if filepath.Ext(event.Name) != ".md" {
				continue
			}

			path := event.Name

			// Log saved event immediately on each write.
			_ = eventlog.Append(d.cfg.DB, eventlog.EventSaved, path, nil)

			// Reset (or start) the debounce timer for this path.
			mu.Lock()
			if t, exists := timers[path]; exists {
				t.Reset(d.cfg.DebounceInterval)
			} else {
				timers[path] = time.AfterFunc(d.cfg.DebounceInterval, func() {
					mtime := time.Now().UTC().UnixMilli()
					if err := d.cfg.Queue.Enqueue(path, mtime); err != nil {
						return
					}
					_ = eventlog.Append(d.cfg.DB, eventlog.EventEnqueued, path, nil)
					mu.Lock()
					delete(timers, path)
					mu.Unlock()
				})
			}
			mu.Unlock()

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			// Non-fatal watcher error; log and continue.
			_ = err
		}
	}
}
