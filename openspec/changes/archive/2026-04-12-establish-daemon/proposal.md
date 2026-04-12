## Why

The foundational packages (store, config, notes, index, queue, eventlog) are in place but no runtime process ties them together. The daemon is the long-running process that wires these subsystems into a working system: it watches the filesystem for note changes, enqueues them, flushes the index to SQLite on a timer, and exposes a Unix-socket HTTP control plane for the CLI commands.

## What Changes

- Introduce `internal/daemon` package with four goroutines managed via `context.WithCancel`: `httpServer`, `queueWorker`, `indexFlusher`, `fsWatcher`
- `httpServer`: listens on a configurable Unix socket path using `net/http`; handles `/status`, `/enqueue`, `/apply` HTTP endpoints
- `queueWorker`: stub implementation — dequeues next entry and logs a `processed` event (full AI pipeline deferred to M3)
- `indexFlusher`: timer-based loop that calls `index.FlushDirty` on a configurable interval
- `fsWatcher`: uses `fsnotify` to watch the workspace root; applies configurable debounce before calling `queue.Enqueue` and logging a `saved` then `enqueued` event
- Graceful shutdown: SIGTERM/SIGINT → `cancel()` → drain goroutines → `index.FlushDirty` before exit

## Capabilities

### New Capabilities
- `daemon-lifecycle`: Four-goroutine daemon with context-managed lifecycle, Unix-socket HTTP server, graceful shutdown on signal

### Modified Capabilities
- `note-index`: Expose `FlushDirty` method signature usable by the `indexFlusher` goroutine (already specified; no requirement changes)

## Impact

- New package: `internal/daemon`
- New dependency: `github.com/fsnotify/fsnotify` (already listed in prerequisites)
- Uses existing: `internal/store`, `internal/config`, `internal/index`, `internal/queue`, `internal/eventlog`
- `cmd/annotate` `daemon` subcommand will wire to this package (M1 step 8)
