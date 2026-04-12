## Context

Six of eight M1 packages are complete (store, config, notes, index, queue, eventlog). The daemon is the runtime glue — a long-running process that must coordinate file watching, queue writes, index persistence, and an HTTP control plane. All sub-packages expose synchronous, context-unaware APIs; the daemon is the only place that manages goroutines, contexts, and signals.

The workspace is a directory of Markdown notes. The daemon watches it with `fsnotify`, debounces rapid saves, and enqueues changed paths. A stub worker dequeues entries and logs events (AI calls are M3). An index flusher persists dirty in-memory index entries to SQLite on a timer. An HTTP server on a Unix socket lets the CLI communicate with the running daemon.

## Goals / Non-Goals

**Goals:**
- Launch four goroutines under a single `context.WithCancel` root
- `fsWatcher`: debounce fsnotify events → `queue.Enqueue` + `eventlog.Append` (`saved`, `enqueued`)
- `queueWorker`: dequeue loop → log `processed` event (stub, no AI)
- `indexFlusher`: timer-based `index.FlushDirty` calls
- `httpServer`: Unix socket, `net/http`, `/status` endpoint
- Graceful shutdown: signal → cancel → drain goroutines (with timeout) → final `FlushDirty`
- Tests validate goroutine start/stop, debounce behavior, and queue-to-eventlog path

**Non-Goals:**
- AI pipeline, redaction, merge logic (M3)
- `/enqueue`, `/apply` HTTP endpoints (M1 step 8 / M5)
- Per-namespace policy, retry logic, conflict detection (M4)
- `annotate daemon` cobra wiring (M1 step 8)

## Decisions

### Goroutine lifecycle: `errgroup` vs manual `sync.WaitGroup`

**Decision**: Use `golang.org/x/sync/errgroup` with a shared context.

**Rationale**: `errgroup.WithContext` provides a derived context that cancels on first goroutine error and a `Wait()` that collects errors — exactly the semantics needed. Manual `WaitGroup` + channel plumbing is equivalent but more boilerplate. All four goroutines should be treated as peers; if any exits unexpectedly the others should stop.

**Alternative considered**: `sync.WaitGroup` + separate error channel. Rejected: requires more boilerplate and doesn't auto-cancel siblings on failure.

### Debounce strategy: timer reset per path

**Decision**: Maintain a `map[string]*time.Timer` (one timer per file path) protected by a mutex. On each fsnotify event for path P, reset P's timer to `debounce_interval`. When the timer fires, enqueue P.

**Rationale**: Per-path debounce means rapid saves to file A don't delay unrelated file B. Matches the implementation plan's intent (`configurable debounce → enqueue`).

**Alternative considered**: Single shared timer reset on any event. Rejected: a flurry of saves to one file could delay all other files indefinitely.

### HTTP transport: Unix socket

**Decision**: Listen on a Unix domain socket at a configurable path (default `.annotate/daemon.sock`). Use `net.Listen("unix", socketPath)` and serve with `http.Server`.

**Rationale**: Unix sockets avoid port conflicts, are naturally local-only (no firewall rules needed), and allow the CLI to detect a running daemon by attempting a connect before falling back to a non-zero exit.

**Alternative considered**: TCP on localhost with a fixed port. Rejected: port conflicts possible; harder to detect stale/absent daemon.

### Shutdown sequencing

**Decision**: On SIGTERM or SIGINT:
1. Cancel the root context (signals all goroutines to stop)
2. `errgroup.Wait()` with a 10-second hard timeout
3. Call `index.FlushDirty` unconditionally after goroutines exit

**Rationale**: FlushDirty must run after the indexFlusher goroutine stops to avoid a data race on the dirty set. A hard timeout prevents indefinite hang if a goroutine is blocked on a slow DB write.

### queueWorker: blocking vs polling

**Decision**: The worker polls `queue.Dequeue()` in a loop with a short sleep (100ms) when the queue is empty.

**Rationale**: The queue API is synchronous (no blocking channel). A short sleep keeps CPU use negligible when idle. In M3 this will be replaced by a channel-notified design, but for the stub the simplicity is appropriate.

**Alternative considered**: `time.Ticker` with longer interval. Rejected: higher latency for the M1 verification test.

## Risks / Trade-offs

- **Timer leak on rapid file churn**: If hundreds of new paths arrive before any timer fires, the debounce map grows unbounded. → Mitigation: acceptable for M1 (personal note corpus is small); a size cap can be added in M4.
- **Unix socket stale file**: If the daemon crashes, the socket file remains and the next start fails. → Mitigation: remove the socket file on startup before binding (`os.Remove` before `net.Listen`); log a warning.
- **errgroup dependency**: Adds `golang.org/x/sync` to go.mod. → Already a common Go dependency; no concern.
- **Test flakiness from real timers**: Debounce tests using real `time.Sleep` can be slow or flaky under load. → Mitigation: inject a clock/ticker interface for unit tests; integration tests use a short debounce interval (1ms).
