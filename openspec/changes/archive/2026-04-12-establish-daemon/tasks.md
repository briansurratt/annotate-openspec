## 1. Package scaffolding and dependencies

- [x] 1.1 Add `golang.org/x/sync` to go.mod (`go get golang.org/x/sync`)
- [x] 1.2 Add `github.com/fsnotify/fsnotify` to go.mod if not already present
- [x] 1.3 Create `internal/daemon/daemon.go` with `Config` struct and `Daemon` type skeleton

## 2. Daemon construction

- [x] 2.1 Write failing test: `New` returns error on nil DB
- [x] 2.2 Implement `daemon.New(cfg Config) (*Daemon, error)` — validate config fields, store references

## 3. HTTP server (Unix socket)

- [x] 3.1 Write failing test: socket file exists within 100ms of `Run`
- [x] 3.2 Implement `httpServer` goroutine: remove stale socket, `net.Listen("unix", ...)`, serve `net/http`
- [x] 3.3 Write failing test: GET `/status` returns 200 with `{"status":"ok"}`
- [x] 3.4 Implement `/status` handler
- [x] 3.5 Write failing test: socket file removed after context cancel
- [x] 3.6 Implement socket cleanup on shutdown (`defer os.Remove`)

## 4. fsWatcher with debounce

- [x] 4.1 Write failing test: single `.md` save produces one enqueue after debounce interval
- [x] 4.2 Implement `fsWatcher` goroutine: `fsnotify.NewWatcher`, watch workspace root, per-path timer map with mutex
- [x] 4.3 Write failing test: rapid saves deduplicate (three writes → one enqueue)
- [x] 4.4 Implement debounce timer reset logic
- [x] 4.5 Write failing test: non-`.md` file saves are ignored
- [x] 4.6 Implement extension filter in watcher event handler
- [x] 4.7 Write failing test: saves to file A and file B each enqueue independently
- [x] 4.8 Verify per-path timer map handles concurrent paths correctly

## 5. queueWorker stub

- [x] 5.1 Write failing test: pending entry is dequeued and `processed` event logged
- [x] 5.2 Implement `queueWorker` goroutine: poll loop with 100ms sleep on empty queue, dequeue → `eventlog.Append(Processed)` → `queue.Remove`
- [x] 5.3 Write failing test: empty queue produces no events
- [x] 5.4 Write failing test: worker exits cleanly on context cancel

## 6. indexFlusher

- [x] 6.1 Write failing test: `FlushDirty` called at least once within 3× flush interval
- [x] 6.2 Implement `indexFlusher` goroutine: `time.NewTicker`, call `index.FlushDirty` on each tick
- [x] 6.3 Write failing test: `FlushDirty` called once on shutdown after loop exits
- [x] 6.4 Implement final flush in goroutine cleanup path

## 7. Daemon.Run and graceful shutdown

- [x] 7.1 Write failing test: all four goroutines start within 100ms of `Run`
- [x] 7.2 Implement `Daemon.Run(ctx context.Context) error` using `errgroup.WithContext`; launch all four goroutines
- [x] 7.3 Write failing test: context cancel causes `Run` to return nil within 1 second
- [x] 7.4 Write failing test: goroutine error propagates and cancels siblings
- [x] 7.5 Implement 10-second drain timeout in `Run` shutdown path
- [x] 7.6 Write failing test: final `FlushDirty` called after all goroutines stop
- [x] 7.7 Implement post-`Wait` final flush in `Run`

## 8. Integration verification

- [x] 8.1 Write integration test: start daemon, save a `.md` file, verify queue row and `enqueued` event log entry exist
- [x] 8.2 Verify `go test ./internal/daemon/...` passes with race detector (`-race`)
