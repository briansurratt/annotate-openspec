## ADDED Requirements

### Requirement: Root command
The system SHALL provide an `annotate` binary built from `cmd/annotate/main.go` with a `cobra` root command that accepts `--config` flag (default: `~/.annotate/config.yaml`) and delegates all work to subcommands.

#### Scenario: Help output lists all subcommands
- **WHEN** `annotate --help` is run
- **THEN** the output lists `daemon`, `enqueue`, `apply`, `report`, `status`, and `index` subcommands

#### Scenario: Unknown subcommand returns non-zero exit
- **WHEN** `annotate unknown-cmd` is run
- **THEN** the process exits with a non-zero exit code
- **AND** an error message is printed to stderr

### Requirement: daemon subcommand
The system SHALL start the annotate daemon when `annotate daemon` is called.

#### Scenario: Daemon starts and blocks
- **WHEN** `annotate daemon` is run with a valid config
- **THEN** the daemon loads config, opens the SQLite store, constructs a `*daemon.Daemon`, and calls `Daemon.Run(ctx)`
- **AND** the process blocks until a SIGTERM or SIGINT signal is received

#### Scenario: Daemon exits on SIGTERM
- **WHEN** `annotate daemon` is running and receives SIGTERM
- **THEN** the daemon context is cancelled
- **AND** the process exits with exit code 0 after graceful shutdown

#### Scenario: Daemon fails on invalid config
- **WHEN** `annotate daemon` is run with a missing or invalid config file
- **THEN** the process exits with a non-zero exit code
- **AND** an error message describing the config problem is printed to stderr

### Requirement: enqueue subcommand
The system SHALL send an enqueue request to the running daemon when `annotate enqueue <file>` is called.

#### Scenario: Successful enqueue
- **WHEN** `annotate enqueue <file>` is run and the daemon is running
- **THEN** an HTTP POST is made to `/enqueue` on the daemon's Unix socket with the file path in the request body
- **AND** the process exits with exit code 0

#### Scenario: Daemon not running
- **WHEN** `annotate enqueue <file>` is run and no daemon socket file exists or the connection is refused
- **THEN** a clear error message is printed to stderr (e.g., "daemon is not running")
- **AND** the process exits with a non-zero exit code

#### Scenario: Daemon enqueue endpoint receives the path
- **WHEN** the daemon receives POST `/enqueue` with a file path in the request body
- **THEN** the daemon calls `queue.Enqueue` with that path
- **AND** responds with HTTP 200

### Requirement: apply subcommand stub
The system SHALL provide an `annotate apply <file>` subcommand that stubs the apply operation for M1.

#### Scenario: Apply prints stub message
- **WHEN** `annotate apply <file>` is run
- **THEN** the output contains "not yet implemented"
- **AND** the process exits with exit code 0

### Requirement: report subcommand stub
The system SHALL provide an `annotate report` subcommand that stubs the report operation for M1.

#### Scenario: Report prints stub message
- **WHEN** `annotate report` is run
- **THEN** the output contains "not yet implemented"
- **AND** the process exits with exit code 0

### Requirement: status subcommand stub
The system SHALL provide an `annotate status` subcommand that stubs the status operation for M1.

#### Scenario: Status prints stub message
- **WHEN** `annotate status` is run
- **THEN** the output contains "not yet implemented"
- **AND** the process exits with exit code 0

### Requirement: index rebuild subcommand stub
The system SHALL provide an `annotate index rebuild` subcommand that stubs the index rebuild operation for M1.

#### Scenario: Index rebuild prints stub message
- **WHEN** `annotate index rebuild` is run
- **THEN** the output contains "not yet implemented"
- **AND** the process exits with exit code 0
