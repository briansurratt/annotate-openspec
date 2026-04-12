## 1. Daemon HTTP enqueue endpoint

- [x] 1.1 Write failing test for POST `/enqueue` handler: verify it calls `queue.Enqueue` with the request body path and returns HTTP 200
- [x] 1.2 Add POST `/enqueue` handler to `internal/daemon` HTTP mux
- [x] 1.3 Commit: `feat(daemon): add /enqueue HTTP handler`

## 2. Root command and package scaffold

- [x] 2.1 Create `cmd/annotate/` directory and `main.go` with cobra root command; add `--config` flag defaulting to `~/.annotate/config.yaml`
- [x] 2.2 Write test: `annotate --help` output includes all six subcommand names
- [x] 2.3 Write test: unknown subcommand exits non-zero
- [ ] 2.4 Commit: `feat(cmd): scaffold cobra root command`

## 3. daemon subcommand

- [x] 3.1 Write integration test: `annotate daemon` with valid config starts daemon and exits 0 on SIGTERM
- [x] 3.2 Write test: `annotate daemon` with missing config exits non-zero with error message on stderr
- [x] 3.3 Implement `cmd/annotate/daemon.go`: load config → open store → construct `daemon.Daemon` → call `Run(ctx)` with signal-based cancellation
- [ ] 3.4 Commit: `feat(cmd): add daemon subcommand`

## 4. enqueue subcommand

- [x] 4.1 Write test: `annotate enqueue <file>` with daemon running sends POST `/enqueue` and exits 0
- [x] 4.2 Write test: `annotate enqueue <file>` with daemon not running prints "daemon is not running" to stderr and exits non-zero
- [x] 4.3 Implement `cmd/annotate/enqueue.go`: load config for socket path → dial Unix socket → POST `/enqueue` with file path; check connection error for non-zero exit
- [ ] 4.4 Commit: `feat(cmd): add enqueue subcommand`

## 5. Stub subcommands

- [x] 5.1 Implement `cmd/annotate/apply.go` stub: print "not yet implemented" and exit 0
- [x] 5.2 Implement `cmd/annotate/report.go` stub: print "not yet implemented" and exit 0
- [x] 5.3 Implement `cmd/annotate/status.go` stub: print "not yet implemented" and exit 0
- [x] 5.4 Implement `cmd/annotate/index.go` with `rebuild` subcommand stub: print "not yet implemented" and exit 0
- [x] 5.5 Write tests: each stub subcommand prints "not yet implemented" and exits 0
- [ ] 5.6 Commit: `feat(cmd): add apply, report, status, index rebuild stub subcommands`

## 6. M1 end-to-end verification

- [ ] 6.1 Run `annotate daemon` and save a `.md` file; verify a queue row and `enqueued` event log entry exist in SQLite
- [ ] 6.2 Run `go build ./cmd/annotate/` and confirm the binary compiles without errors
- [ ] 6.3 Run full test suite (`go test ./...`) and confirm all tests pass
