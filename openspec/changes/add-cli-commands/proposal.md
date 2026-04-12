## Why

The daemon, queue, and event log subsystems (M1.1–M1.7) are complete but have no user-facing entry points. A CLI layer is needed to start the daemon, trigger enqueue operations, apply accepted suggestions, check status, and view reports — making the system usable end-to-end for M1 verification.

## What Changes

- Add `cmd/annotate/` package with a `cobra`-based root command and six subcommands
- `annotate daemon` — starts the background daemon (HTTP server + goroutines)
- `annotate enqueue <file>` — sends a file to the daemon's queue via Unix socket HTTP; exits non-zero if daemon is not running
- `annotate apply <file>` — sends an apply request to the daemon via Unix socket HTTP (stub in M1)
- `annotate report` — fetches and renders metrics from the daemon (stub in M1)
- `annotate status` — fetches queue depth and worker state from the daemon (stub in M1)
- `annotate index rebuild` — triggers an index rebuild on the daemon (stub in M1)

## Capabilities

### New Capabilities
- `cli-commands`: Cobra-wired CLI entry point with `daemon`, `enqueue`, `apply`, `report`, `status`, and `index rebuild` subcommands; `enqueue` enforces daemon-running check with non-zero exit

### Modified Capabilities

## Impact

- New package: `cmd/annotate/` (main entrypoint + subcommand files)
- Depends on: `internal/daemon` (to start it), `internal/config` (to load config), and Unix socket HTTP client for IPC subcommands
- No changes to existing `internal/` packages in M1; stubs for `apply`, `report`, `status`, `index rebuild` will be filled in M5
