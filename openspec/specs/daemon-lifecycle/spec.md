### Requirement: Daemon construction
The system SHALL construct a `Daemon` by accepting a `Config` struct containing a `*sql.DB`, an `*index.Index`, a `*queue.Queue`, the workspace root path, the Unix socket path, the debounce interval, and the index flush interval.

#### Scenario: Successful construction
- **WHEN** `daemon.New(cfg)` is called with a fully populated `Config`
- **THEN** a `*Daemon` is returned with no goroutines started
- **AND** no error is returned

#### Scenario: Construction with missing DB
- **WHEN** `daemon.New(cfg)` is called with a nil `*sql.DB`
- **THEN** an error is returned

### Requirement: Daemon start
The system SHALL start all four goroutines when `Daemon.Run(ctx)` is called, and block until all goroutines exit.

#### Scenario: All goroutines start
- **WHEN** `Daemon.Run(ctx)` is called with a live context
- **THEN** the httpServer, queueWorker, indexFlusher, and fsWatcher goroutines are all running within 100ms

#### Scenario: Context cancellation stops all goroutines
- **WHEN** the context passed to `Daemon.Run` is cancelled
- **THEN** all four goroutines exit cleanly
- **AND** `Daemon.Run` returns nil

#### Scenario: Goroutine failure propagates
- **WHEN** one goroutine returns a non-nil error
- **THEN** the context is cancelled for all sibling goroutines
- **AND** `Daemon.Run` returns that error

### Requirement: HTTP server on Unix socket
The daemon SHALL start a `net/http` server listening on the configured Unix socket path.

#### Scenario: Socket file created on start
- **WHEN** `Daemon.Run` is called
- **THEN** a Unix socket file exists at the configured socket path within 100ms

#### Scenario: Stale socket removed on start
- **WHEN** a socket file already exists at the configured path before `Daemon.Run` is called
- **THEN** the existing file is removed before binding
- **AND** the server starts successfully

#### Scenario: Status endpoint responds
- **WHEN** a GET request is made to `/status` on the Unix socket
- **THEN** a 200 response is returned with a JSON body containing `"status": "ok"`

#### Scenario: Socket file removed on stop
- **WHEN** the context is cancelled and the HTTP server shuts down
- **THEN** the Unix socket file is removed from the filesystem

### Requirement: fsWatcher with debounce
The daemon SHALL watch the workspace root for filesystem events and enqueue changed `.md` file paths after a configurable debounce interval.

#### Scenario: Save event enqueues after debounce
- **WHEN** a `.md` file is written in the workspace root
- **AND** no further writes to that file occur within the debounce interval
- **THEN** exactly one `queue.Enqueue` call is made for that file path
- **AND** a `saved` event and an `enqueued` event are appended to the event log

#### Scenario: Rapid saves deduplicate via debounce
- **WHEN** a `.md` file is written three times within the debounce interval
- **THEN** exactly one `queue.Enqueue` call is made for that path (timer resets on each write)

#### Scenario: Non-markdown files are ignored
- **WHEN** a `.txt` or `.go` file is written in the workspace root
- **THEN** no `queue.Enqueue` call is made

#### Scenario: Debounce is per-path
- **WHEN** file A and file B are each written once, within the debounce interval of each other
- **THEN** both file A and file B are independently enqueued after their respective debounce timers fire

### Requirement: queueWorker stub
The daemon SHALL run a queue worker goroutine that continuously dequeues pending entries and logs a `processed` event for each.

#### Scenario: Entry dequeued and logged
- **WHEN** a path is present in the queue with status `pending`
- **THEN** the worker calls `queue.Dequeue`, calls `eventlog.Append` with event type `processed`, and calls `queue.Remove`

#### Scenario: Empty queue yields no events
- **WHEN** the queue is empty
- **THEN** the worker does not call `eventlog.Append` and does not error

#### Scenario: Worker exits on context cancellation
- **WHEN** the context is cancelled while the worker is in its idle poll loop
- **THEN** the worker goroutine exits cleanly

### Requirement: indexFlusher timer loop
The daemon SHALL periodically call `index.FlushDirty` on a configurable interval to persist dirty index entries to SQLite.

#### Scenario: FlushDirty called on interval
- **WHEN** the indexFlusher goroutine is running with a 50ms interval
- **THEN** `FlushDirty` is called at least once within 150ms

#### Scenario: FlushDirty called on shutdown
- **WHEN** the context is cancelled
- **THEN** `FlushDirty` is called exactly once after the goroutine loop exits, before the goroutine returns

#### Scenario: Flusher exits on context cancellation
- **WHEN** the context is cancelled
- **THEN** the indexFlusher goroutine exits cleanly after the final flush

### Requirement: Graceful shutdown sequence
The daemon SHALL perform a coordinated shutdown when the root context is cancelled, ensuring the index is flushed after all goroutines exit.

#### Scenario: Goroutines drain within timeout
- **WHEN** the context is cancelled
- **THEN** all goroutines exit within 10 seconds
- **AND** `Daemon.Run` returns after all goroutines have stopped

#### Scenario: Final index flush after goroutines stop
- **WHEN** all goroutines have exited after context cancellation
- **THEN** `index.FlushDirty` is called once more by the shutdown sequence to capture any last dirty entries
