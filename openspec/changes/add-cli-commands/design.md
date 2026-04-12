## Context

M1.1–M1.7 produced `internal/daemon`, `internal/queue`, `internal/eventlog`, `internal/index`, `internal/config`, and `internal/store`. The daemon exposes a Unix socket HTTP server with a `/status` endpoint. All subsystems are usable from Go code but have no CLI entry point. M1.8 adds `cmd/annotate/` to wire everything together via `cobra`.

## Goals / Non-Goals

**Goals:**
- Provide a `cobra`-based root command and six subcommands: `daemon`, `enqueue`, `apply`, `report`, `status`, `index rebuild`
- `daemon` subcommand loads config, opens the store, constructs and runs the daemon
- `enqueue` subcommand contacts the running daemon via Unix socket HTTP; exits non-zero if the daemon is not reachable
- `apply`, `report`, `status`, `index rebuild` are functional stubs that print "not yet implemented" — they will be filled in M5
- One binary (`annotate`) built from `cmd/annotate/main.go`

**Non-Goals:**
- Full implementation of `apply`, `report`, `status`, or `index rebuild` (M5)
- Daemon auto-restart or process supervision
- Shell completion or man page generation

## Decisions

**Decision: One file per subcommand under `cmd/annotate/`**
Each subcommand lives in its own file (`daemon.go`, `enqueue.go`, `apply.go`, etc.) in package `main`. This keeps the root `cmd.go` thin and makes each subcommand easy to locate and extend. Alternative (single file) becomes unwieldy as commands grow.

**Decision: Unix socket path from config, defaulting to `~/.annotate/annotate.sock`**
The socket path is loaded from `internal/config` so it matches the path the daemon bound to. The `enqueue` command (and future IPC commands) derive the socket from the same config, guaranteeing they talk to the right daemon instance. Hard-coding the path would break multi-workspace setups.

**Decision: `enqueue` uses HTTP POST `/enqueue` on the Unix socket**
HTTP over a Unix socket is already the daemon's IPC mechanism (established in M1.7). Reusing it for `enqueue` avoids introducing a second IPC channel. The daemon needs a new `/enqueue` handler added to its HTTP mux. Alternative (stdin pipe, signals) would require a new protocol.

**Decision: Non-zero exit for `enqueue` when daemon is unreachable**
The implementation plan explicitly requires this. Implemented by attempting a dial on the Unix socket before sending the request; a connection-refused or no-such-file error prints a clear message and calls `os.Exit(1)`.

**Decision: Stubs for `apply`, `report`, `status`, `index rebuild` print a message and exit 0**
These commands need to exist so the CLI surface is complete and discoverable, but their backends (M5 metrics, apply logic) don't exist yet. Stubs avoid confusion and make `--help` accurate without blocking M1 verification.

## Risks / Trade-offs

[Risk: `/enqueue` HTTP handler not yet on the daemon] → Add the handler to `internal/daemon` as part of this change. It's a small addition that fits naturally alongside the existing `/status` handler.

[Risk: Config loading path differs between `daemon` start and `enqueue` client] → Both commands call the same `config.Load()` function with the same default path (`~/.annotate/config.yaml`) so the socket path is always consistent.

[Risk: `cmd/annotate/` package `main` makes unit testing harder] → Subcommand logic is thin wrappers; all testable logic lives in `internal/` packages. Integration tests verify CLI behavior via subprocess.
