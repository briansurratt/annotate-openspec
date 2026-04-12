## 1. Schema Migration

- [x] 1.1 Write a failing test that asserts the `queue` table has `position`, `status`, `created_at`, and `updated_at` columns after migration
- [x] 1.2 Update the `queue` table migration in `internal/store` to include `position INTEGER NOT NULL`, `status TEXT NOT NULL DEFAULT 'pending'`, `created_at INTEGER NOT NULL`, `updated_at INTEGER NOT NULL`
- [x] 1.3 Add a unique partial index on `(file_path)` WHERE `status = 'pending'` and an index on `(status, position)` to the migration
- [x] 1.4 Verify migration test passes; commit

## 2. Queue Package Scaffold

- [x] 2.1 Create `internal/queue/queue.go` with the `Queue` struct holding `*sql.DB` and four `*sql.Stmt` fields
- [x] 2.2 Write a failing test for `queue.New(db)` — asserts non-nil return and no error with a valid in-memory test DB
- [x] 2.3 Implement `queue.New(db *sql.DB) (*Queue, error)` — prepare all four statements, return error on any failure
- [x] 2.4 Verify construction tests pass; commit

## 3. Enqueue

- [x] 3.1 Write failing tests for `Enqueue`: new path inserts a row; duplicate pending path updates `mtime` without adding a row; path with `processing` status inserts a new `pending` row
- [x] 3.2 Implement `(q *Queue) Enqueue(path string, mtime int64) error` using an INSERT OR IGNORE + UPDATE strategy (or UPSERT) respecting the partial unique index
- [x] 3.3 Verify all enqueue tests pass; commit

## 4. Dequeue

- [x] 4.1 Write failing tests for `Dequeue`: returns oldest pending entry and marks it `processing`; returns `nil, nil` when queue is empty
- [x] 4.2 Define `Entry` struct with `ID int64`, `FilePath string`, `Mtime int64`
- [x] 4.3 Implement `(q *Queue) Dequeue() (*Entry, error)` — SELECT lowest-position pending row, UPDATE status to `processing`, return entry
- [x] 4.4 Verify dequeue tests pass; commit

## 5. ReEnqueue

- [x] 5.1 Write failing tests for `ReEnqueue`: moves a processing row to pending at back of queue; returns error for non-existent ID
- [x] 5.2 Implement `(q *Queue) ReEnqueue(id int64) error` — UPDATE status to `pending` and set `position` to `(SELECT MAX(position) + 1 FROM queue)` for the given ID; return error if no row affected
- [x] 5.3 Verify re-enqueue tests pass; commit

## 6. Remove

- [x] 6.1 Write failing tests for `Remove`: deletes an existing row; returns error for non-existent ID
- [x] 6.2 Implement `(q *Queue) Remove(id int64) error` — DELETE row by ID, return error if no row affected
- [x] 6.3 Verify remove tests pass; commit

## 7. Integration Verification

- [x] 7.1 Write an integration test that runs a full FIFO cycle: enqueue A, enqueue B, dequeue (gets A), remove A, dequeue (gets B), remove B — empty queue
- [x] 7.2 Write a dedup integration test: enqueue path X twice, assert only one row exists with updated mtime, dequeue returns X once
- [x] 7.3 Write a conflict cycle test: enqueue, dequeue (marks processing), re-enqueue, dequeue again (gets it back at back)
- [x] 7.4 Verify all integration tests pass; commit
