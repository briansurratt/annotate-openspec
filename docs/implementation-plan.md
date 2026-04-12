# Implementation Plan: Automated Note Annotation

Module: `github.com/briansurratt/annotate`

## Decisions

- **Link cap defaults**: use §11 values as-is (`daily.journal.*`: 8, `projects.*`: 12, `reference.*`: 8, `technology.*`: 10)
- **Per-namespace policy overrides**: open-ended glob map — any namespace, not just the five listed
- **Debounce**: configurable via `debounce_interval` in `.annotate/config.yaml` (default: 500ms); tune based on M6 `conflict_discards` data

## Prerequisites

- Go 1.21+
- `go mod init github.com/briansurratt/annotate`
- Key deps: `cobra`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`, `github.com/fsnotify/fsnotify`, `invopop/jsonschema`, `github.com/anthropics/anthropic-sdk-go`, `github.com/openai/openai-go`

---

## M1 — Foundation

**Goal**: Save a file → correct queue and event log entries exist.

1. **`internal/store`** — open SQLite with WAL mode, schema migrations for `queue`, `event_log`, `index_cache`, `metrics` tables
2. **`internal/config`** — YAML config struct, load + validate (fail on unknown keys), `config.yaml` schema covering all §13 PRD fields including `debounce_interval`
3. **`internal/notes`** — frontmatter parse (`yaml.v3`), three-part document model (pre-fence / frontmatter map / raw body), atomic write (`<file>.tmp` → `os.Rename`), content hash (body + non-`ai_suggestions` frontmatter keys)
4. **`internal/index`** — `NoteEntry` struct, in-memory map, startup mtime scan vs SQLite cache, dirty tracking for `indexFlusher`
5. **`internal/queue`** — SQLite-backed FIFO deduplicating queue: enqueue (deduplicate → append to back with updated mtime), dequeue front, re-enqueue (conflict path), remove on success
6. **`internal/eventlog`** — append-only SQLite writes for all event types (`saved`, `enqueued`, `skipped_excluded_namespace`, `conflict_discarded`, `retry_needed`, `processed`), configurable retention
7. **`internal/daemon`** — four goroutines wired with `context.WithCancel`: `httpServer` (Unix socket, `net/http`), `queueWorker` (stub — dequeue/log only), `indexFlusher` (timer-based dirty flush), `fsWatcher` (fsnotify + configurable debounce → enqueue)
8. **`cmd/annotate`** — `cobra` wiring: `daemon`, `enqueue`, `apply`, `report`, `status`, `index rebuild` subcommands; `enqueue` fails with non-zero exit if daemon not running

**Verification**: `annotate daemon` starts; saving a `.md` file produces a queue row and `enqueued` event log entry.

---

## M2 — Data Sanitization

**Goal**: Excluded-namespace files never reach prompt assembly; redacted content visible in dry-run.

1. **`internal/redact`** — `Redactor` struct: patterns compiled once at startup, `Redact(s string) string` pure function; default patterns for phone, email, URL; configurable marker
2. **Namespace exclusion** in worker stub — check file namespace against configured exclude globs before any processing; emit `skipped_excluded_namespace` event; no AI call
3. **Unit tests** — redaction (fixture corpus), namespace exclusion edge cases

**Verification**: saving a `people.*` file emits `skipped_excluded_namespace`; `--dry-run=prompt` on a note with PII shows `[REDACTED]` placeholders.

---

## M3 — AI Integration

**Goal**: Saving a file populates `ai_suggestions` with links and tags.

1. **`internal/provider`** — `Provider` interface (`Name()`, `MaxContextTokens()`, `Suggest()`); `ProviderError` with `Retryable` field; Anthropic adapter (forced tool use); OpenAI/GitHub Models adapter (`response_format: json_schema`); `invopop/jsonschema` generates schema from Go structs at startup
2. **`ai_suggestions` schema structs** in `internal/notes` — `AISuggestions`, `SuggestionMeta`, `LinkSuggestion`, `TagSuggestion`, `TagSuggestions`, `SuggestionStatus` constants; `ProviderOutput` (no `meta`/`status` fields — worker populates these)
3. **Prompt assembly** in `internal/worker` — priority-ordered context sections; token budget via `len/4` heuristic; mtime-sorted index truncation; tag catalog with frequency + 3 sample titles; link graph backlink counts inlined into index entries

   Context priority order (truncate from bottom up):
   1. Policy constraints — never truncated
   2. Current note (body + meta) — never truncated
   3. Prior suggestions — never truncated
   4. Tag catalog (full) — never truncated
   5. Link graph (inline in index entries)
   6. Title/alias index
   7. Section heading index
   8. Note index (mtime-sorted, tail trimmed first, default cap 500 entries)
   9. Adjacent notes (daily journal only, trimmed first)

4. **`internal/merge`** — pure `MergeSuggestions(prior, fresh, index)` implementing all 6 rule-application steps:
   1. Build lookup maps from prior
   2. For each link/tag in fresh: if prior status is `accepted`, `rejected`, or `applied` → carry forward unchanged; if `proposed` or missing → use fresh with `status: proposed`
   3. `accepted`/`applied` items in prior not in fresh → carry forward permanently
   4. `rejected` items in prior not in fresh → carry forward (never re-surfaced)
   5. `proposed` items whose link target no longer exists → set `status: target_missing`
   6. `rejected` items whose target disappears → stay `rejected` (author decision preserved)
5. **Worker full pipeline** — dequeue → namespace check → hash check (skip if unchanged) → redact → assemble prompt → call provider → validate output (confidence ≥ 0.50, link targets exist in index, dedup) → merge → atomic write → `processed` event
6. **Own-write suppression** — hash excludes `ai_suggestions` key; daemon's own atomic write produces no AI call on the follow-up fsnotify event
7. **Startup health check** — config valid → workspace readable → SQLite ok → `.annotate/` writable → stale socket check → provider ping (warning only, not fatal)

**Verification**: `annotate daemon` + save a note → `ai_suggestions` block written with links and tags.

---

## M4 — Resilience

**Goal**: AI unavailability handled gracefully; namespace policies applied correctly.

1. **Retry loop** in worker — exponential backoff with jitter (2s→4s→8s→16s→32s, max 60s); `retry_needed` event; configurable max attempts (0 = unlimited); non-retryable errors (`401`, `400`, schema failure) remove from queue and write `meta.last_error`
2. **Pre-write conflict check** — compare file mtime at write time vs enqueue time; on conflict: discard result, re-enqueue at back, increment `conflict_discards`
3. **Per-namespace policy overrides** — open-ended glob map in config; applied during prompt assembly (max links, max tags, adjacent notes count)
4. **Adjacent daily notes** — filename-based date parsing, configurable date format per namespace (Go time format), read bodies from disk at prompt time (not cached), run through `Redactor`
5. **Dry-run modes** — `--dry-run=prompt` (print assembled prompt, no AI call, no write); `--dry-run=full` (AI call, print merged YAML, no write)
6. **Lazy index flush** — `indexFlusher` goroutine on configurable timer; `FlushNow()` called on graceful shutdown
7. **Graceful shutdown** — SIGTERM → `cancel()` → drain worker (10s timeout) → `indexFlusher.FlushNow()`

**Verification**: kill provider mid-run → file stays at queue front; restore provider → processing resumes; journal note includes preceding daily note bodies in prompt.

---

## M5 — Apply Action and Reporting

**Goal**: Accepted tag suggestions promote to top-level metadata; metrics report renders correctly.

1. **`annotate apply <file>`** — HTTP call to daemon; daemon reads note, finds `accepted` tag suggestions, promotes to top-level `tags` frontmatter, sets status → `applied`, atomic write
2. **`internal/metrics`** — counters written to SQLite on each event: suggestion counts by type, acceptance/rejection rates, `target_missing` count, avg confidence (accepted), median latency, `conflict_discards`, AI errors/retries, namespace skips
3. **`annotate report`** — HTTP call to daemon; renders metrics over configurable retention window
4. **`annotate status`** — queue depth, worker state (idle/processing), last processed file

**Verification**: accept a tag suggestion → `annotate apply <file>` → top-level `tags` updated; `annotate report` prints populated metrics.

---

## M6 — Quality Tuning (Evaluation Phase)

**Goal**: Evidence-based decisions on thresholds, debounce, provider, and continuous-watch mode.

1. Run against representative note corpus; evaluate suggestion quality
2. Tune per-namespace confidence thresholds and suggestion limits based on `annotate report` data
3. Evaluate `conflict_discards` signal — adjust `debounce_interval` in config if warranted
4. Compare provider quality (Anthropic vs OpenAI vs GitHub Models)
5. Assess `annotate daemon --watch` continuous mode for batch/non-editor workflows (currently deferred)

---

## Package Layout

```
cmd/annotate/          # main entrypoint, cobra command wiring
internal/
  daemon/              # HTTP server, goroutine lifecycle, shutdown
  queue/               # SQLite-backed FIFO deduplicating queue
  index/               # in-memory note index, SQLite cache, fsnotify watcher
  worker/              # queue worker loop, retry logic, conflict detection
  provider/            # Provider interface + adapters (anthropic, openai)
  notes/               # note parsing, frontmatter, atomic write, hash
  redact/              # redaction pipeline
  merge/               # suggestion merge rules (pure functions)
  config/              # config struct, YAML loading, validation
  eventlog/            # append-only event log
  metrics/             # counters, report generation
  store/               # *sql.DB setup, schema migrations, WAL config
```

## Package Build Order (bottom-up)

```
store → config → notes → redact → index → queue → eventlog → metrics
provider → merge → worker → daemon → cmd/annotate
```
