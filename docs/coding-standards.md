# Coding Standards and Practices

Module: `github.com/briansurratt/annotate`

## Go Standards

- **Go Version**: 1.21+
- **Module name**: `github.com/briansurratt/annotate`
- **Code style**: Follow [Effective Go](https://golang.org/doc/effective_go)
- **Linter**: Use `golangci-lint` for code quality
- **Formatting**: `gofmt` for all Go files

## Project Structure

```
annotate/
├── cmd/                    # CLI command entry points
│   └── annotate/
│       └── main.go
├── internal/               # Private packages (not importable externally)
│   ├── store/             # Database and persistence
│   ├── config/            # Configuration loading and validation
│   ├── notes/             # Note parsing and document model
│   ├── index/             # In-memory index and caching
│   ├── queue/             # Processing queue
│   ├── eventlog/          # Event logging
│   ├── daemon/            # Daemon orchestration
│   ├── provider/          # AI provider abstraction
│   ├── worker/            # Processing pipeline
│   ├── merge/             # Suggestion merging logic
│   └── redact/            # Content redaction
├── go.mod                 # Module definition
├── go.sum                 # Dependency checksums
└── README.md
```

## Naming Conventions

- **Packages**: lowercase, single word when possible (no underscores)
- **Functions**: PascalCase for exported, camelCase for private
- **Constants**: ALL_CAPS for package-level constants
- **Variables**: camelCase for local variables
- **Interfaces**: Usually named with `-er` suffix (e.g., `Reader`, `Flusher`, `Provider`)
- **Structs**: PascalCase (e.g., `NoteEntry`, `QueueItem`, `SuggestionMeta`)
- **Database tables**: snake_case (e.g., `event_log`, `index_cache`)
- **YAML keys**: snake_case (e.g., `debounce_interval`, `max_links`)

## Error Handling

- Always check errors explicitly (no silent failures)
- Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Define custom error types for domain-specific errors
- Use error interfaces (`ProviderError` with `Retryable` field) for classified errors
- Log errors with appropriate level and context
- Never use `panic()` for recoverable errors

Example:
```go
type ProviderError struct {
    Message   string
    Retryable bool
}

func process() error {
    result, err := provider.Suggest(ctx, prompt)
    if err != nil {
        return fmt.Errorf("AI suggestion failed: %w", err)
    }
    return nil
}
```

## Functions and Methods

- Keep functions small and focused (single responsibility)
- Return errors as last return value
- Use context.Context as first parameter for long-running operations
- Name receiver variables clearly: `r *Reader`, `s *Store`, `w *Worker`
- Document public functions and types with comments

Example:
```go
// Suggest generates AI suggestions for the given note content.
// It returns an error if the AI provider is unavailable or validation fails.
func (p *Provider) Suggest(ctx context.Context, prompt string) (*ProviderOutput, error) {
    // implementation
}
```

## Testing

### Test-Driven Development (Red → Green → Refactor)

All new features and bug fixes follow TDD. The cycle is mandatory, not optional:

1. **Red** — Write the test first. Run the suite and confirm the new test fails. Verify it fails *for the right reason* (missing function, wrong behavior — not a compile error in the test itself or a misconfigured fixture).
2. **Green** — Write the minimum production code needed to make the failing test pass. Run the full suite; all tests must be green before moving on.
3. **Refactor** — Clean up duplication, improve names, simplify logic. Run the suite after every non-trivial refactor step to confirm nothing regressed.

**Plan and tasks must reflect this order.** A task list for a feature should have the test task appear *before* the implementation task:

```
- [ ] 1. Write tests for Foo (expect red)
- [ ] 2. Implement Foo (expect green)
- [ ] 3. Refactor Foo if needed (expect green throughout)
```

**Verification steps are part of the work.** Each phase ends with running the tests:

```bash
# After writing the test (should see FAIL):
go test ./internal/foo/... -run TestFoo -v

# After implementing (full suite must pass):
go test ./...

# After each refactor step:
go test ./...
```

Never skip the red phase. If a newly written test passes without any production code change, the test is either testing something already implemented or is incorrectly written — stop and investigate before continuing.

### Test Style

- Test files: `*_test.go` in the same package
- Table-driven tests for multiple scenarios
- Test functions: `TestFunctionName(t *testing.T)`
- Subtests: `t.Run()` for grouped test cases
- Mock external dependencies (providers, file system)
- Aim for >80% code coverage for core logic

Example:
```go
func TestMergeSuggestions(t *testing.T) {
    tests := []struct {
        name     string
        prior    *AISuggestions
        fresh    *AISuggestions
        expected *AISuggestions
    }{
        {
            name:  "accepted items preserved",
            prior: &AISuggestions{...},
            fresh: &AISuggestions{...},
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := MergeSuggestions(tt.prior, tt.fresh, tt.index)
            if !reflect.DeepEqual(result, tt.expected) {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## Concurrency

- Use `context.Context` for cancellation and timeouts
- Protect shared state with `sync.Mutex` or `sync.RWMutex`
- Use goroutines with clear ownership and lifecycle
- Always close channels after final send (except receive-only channels)
- Use `context.WithCancel` for graceful shutdown coordination
- Prefer channels over raw synchronization for goroutine communication

Example:
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go func() {
    select {
    case <-ctx.Done():
        return
    case event := <-eventChan:
        process(event)
    }
}()
```

## Database and SQL

- Use prepared statements to prevent SQL injection
- Keep SQL queries in package-level variables or separate files
- Use transactions for multi-statement operations
- Enable WAL mode for SQLite: `PRAGMA journal_mode=WAL`
- Use time.Now().UTC() for timestamps (always UTC)
- Keep database operations in `internal/store` package
- Document table schemas in migration files

### Database Migrations Must Be Atomic

Each migration file must contain a **single logical change**. This ensures easy rollback, debugging, and clear change history.

**Migration Atomicity Guidelines:**
- **One table per migration**: Creating `queue` table = one migration, creating `event_log` table = separate migration
- **One index per migration**: Adding a primary key index = one migration, adding a lookup index = separate migration
- **One schema change per migration**: Altering a column definition = one migration
- **Related constraints in same migration**: Foreign key + index on same new column can be in one migration if they're inseparable

**Migration File Naming:**
```
migrations/
├── 001_create_queue_table.sql
├── 002_create_event_log_table.sql
├── 003_create_index_cache_table.sql
├── 004_create_metrics_table.sql
├── 005_add_queue_index_file_path.sql
├── 006_add_event_log_index_timestamp.sql
├── 007_add_metrics_index_name.sql
```

**Why Atomic Migrations:**
- Easy to rollback individual changes without cascading failures
- Clear git history showing exactly what changed and when
- Reduces risk of partial failures during schema updates
- Simpler to debug issues in production

Example:
```go
const (
    queryInsertEvent = `INSERT INTO event_log (event_type, file_path, details, timestamp) 
                        VALUES (?, ?, ?, ?)`
)

func (s *Store) LogEvent(ctx context.Context, eventType string, filePath string, details string) error {
    stmt, err := s.db.PrepareContext(ctx, queryInsertEvent)
    if err != nil {
        return fmt.Errorf("prepare statement: %w", err)
    }
    defer stmt.Close()
    
    _, err = stmt.ExecContext(ctx, eventType, filePath, details, time.Now().UTC())
    return err
}
```

## Configuration and YAML

- Use `gopkg.in/yaml.v3` for YAML parsing
- Define configuration structs with struct tags for YAML keys
- Validate all configuration on load (fail fast on unknown keys)
- Support environment variable overrides where appropriate
- Document all configuration options in comments

Example:
```go
type Config struct {
    DebounceInterval   time.Duration `yaml:"debounce_interval"`
    MaxRetryAttempts   int           `yaml:"max_retry_attempts"`
    Provider          string         `yaml:"provider"`
    ExcludeNamespaces []string       `yaml:"exclude_namespaces"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }
    
    var cfg Config
    decoder := yaml.NewDecoder(bytes.NewReader(data))
    decoder.KnownFields(true) // Fail on unknown keys
    if err := decoder.Decode(&cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    
    return cfg, nil
}
```

## Comments and Documentation

- Document all exported functions, types, and constants
- Use clear, concise English
- Start comments with the name of the thing being documented
- Explain *why*, not just *what* for complex logic
- Keep inline comments minimal (code should be self-documenting)
- Update comments when code logic changes

Example:
```go
// NoteEntry represents a file in the index with metadata.
// The content hash excludes ai_suggestions to allow detection 
// of own-write changes (daemon reading its own file updates).
type NoteEntry struct {
    Path      string    // Absolute file path
    Mtime     time.Time // Last modified time
    Hash      string    // Content hash (excludes ai_suggestions)
    Metadata  map[string]interface{}
}
```

## File I/O and Atomicity

- Use atomic writes for critical files: write to `.tmp`, then rename
- Always use `os.Rename()` for atomic file swaps
- Call `file.Sync()` before closing long-lived files
- Use `ioutil.TempFile()` for temporary files
- Clean up temporary files on error

Example:
```go
func (n *Note) AtomicWrite(path string) error {
    tmpFile, err := ioutil.TempFile(filepath.Dir(path), ".tmp-*.md")
    if err != nil {
        return fmt.Errorf("create temp file: %w", err)
    }
    defer os.Remove(tmpFile.Name()) // Clean up on error
    
    if err := writeContent(tmpFile, n); err != nil {
        return fmt.Errorf("write content: %w", err)
    }
    
    if err := tmpFile.Sync(); err != nil {
        return fmt.Errorf("sync file: %w", err)
    }
    
    if err := tmpFile.Close(); err != nil {
        return fmt.Errorf("close file: %w", err)
    }
    
    if err := os.Rename(tmpFile.Name(), path); err != nil {
        return fmt.Errorf("rename file: %w", err)
    }
    
    return nil
}
```

## Logging

- Use structured logging where possible (key-value pairs)
- Log at appropriate levels: DEBUG, INFO, WARN, ERROR
- Include context (file path, event type, etc.) in log messages
- Avoid logging sensitive information (API keys, personal data)
- Log errors with full context for debugging

## External Dependencies

Keep dependencies minimal and well-justified:
- `cobra` - CLI framework
- `gopkg.in/yaml.v3` - YAML parsing
- `modernc.org/sqlite` - SQLite driver
- `fsnotify/fsnotify` - File system monitoring
- `invopop/jsonschema` - JSON schema generation
- `anthropics/anthropic-sdk-go` - Anthropic API
- `openai/openai-go` - OpenAI API

Avoid adding dependencies without discussion; prefer standard library solutions.

## Code Review Checklist

Before submitting code:
- [ ] Tests were written *before* the production code (TDD red-green-refactor cycle followed)
- [ ] New test was confirmed to fail before implementation (red phase verified)
- [ ] Full test suite passes after implementation (green phase verified)
- [ ] Refactoring steps kept the suite green throughout
- [ ] Follows naming conventions
- [ ] All errors are handled explicitly
- [ ] Functions have appropriate comments
- [ ] No exported functions without documentation
- [ ] Tests added for new logic (aim >80% coverage)
- [ ] Database operations use prepared statements
- [ ] No hardcoded paths or magic numbers
- [ ] Concurrency uses proper context and synchronization
- [ ] Configuration is validated on load
- [ ] Atomic writes used for critical files
- [ ] No debug logging or temporary code left behind