// Package queue provides a SQLite-backed FIFO deduplicating queue for note
// file paths. Entries are deduplicated by file path: enqueueing an already-pending
// path updates its mtime in place rather than inserting a duplicate row, preserving
// FIFO ordering. A processing row is not affected by a re-enqueue of the same path.
package queue

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	// sqlEnqueue inserts a new pending row or, on conflict with the partial unique
	// index (file_path WHERE status='pending'), updates only the mtime of the
	// existing row — preserving its position in the FIFO order.
	sqlEnqueue = `
		INSERT INTO queue (file_path, mtime, position, status, created_at, updated_at)
		VALUES (?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM queue), 'pending', ?, ?)
		ON CONFLICT(file_path) WHERE status = 'pending'
		DO UPDATE SET mtime = excluded.mtime, updated_at = excluded.updated_at`

	// sqlDequeue atomically selects the oldest pending entry and marks it as
	// processing. Returns the entry via RETURNING. Returns no rows when the queue
	// is empty.
	sqlDequeue = `
		UPDATE queue SET status = 'processing'
		WHERE id = (
			SELECT id FROM queue WHERE status = 'pending' ORDER BY position ASC LIMIT 1
		)
		RETURNING id, file_path, mtime`

	// sqlReEnqueue moves a processing row back to pending at the back of the queue.
	sqlReEnqueue = `
		UPDATE queue
		SET status = 'pending',
		    position = (SELECT COALESCE(MAX(position), 0) + 1 FROM queue)
		WHERE id = ?`

	// sqlRemove deletes a successfully processed queue entry by ID.
	sqlRemove = `DELETE FROM queue WHERE id = ?`
)

// Entry holds the data returned by Dequeue.
type Entry struct {
	ID       int64
	FilePath string
	Mtime    int64
}

// Queue is a SQLite-backed FIFO deduplicating queue.
// All SQL statements are prepared once at construction time and reused.
type Queue struct {
	db            *sql.DB
	stmtEnqueue   *sql.Stmt
	stmtDequeue   *sql.Stmt
	stmtReEnqueue *sql.Stmt
	stmtRemove    *sql.Stmt
}

// New constructs a Queue by preparing all four SQL statements against db.
// Returns an error if any statement fails to prepare; on error all previously
// prepared statements are closed before returning.
func New(db *sql.DB) (*Queue, error) {
	if db == nil {
		return nil, fmt.Errorf("queue.New: db must not be nil")
	}
	q := &Queue{db: db}

	var err error
	if q.stmtEnqueue, err = db.Prepare(sqlEnqueue); err != nil {
		return nil, fmt.Errorf("prepare enqueue: %w", err)
	}
	if q.stmtDequeue, err = db.Prepare(sqlDequeue); err != nil {
		q.stmtEnqueue.Close()
		return nil, fmt.Errorf("prepare dequeue: %w", err)
	}
	if q.stmtReEnqueue, err = db.Prepare(sqlReEnqueue); err != nil {
		q.stmtEnqueue.Close()
		q.stmtDequeue.Close()
		return nil, fmt.Errorf("prepare re-enqueue: %w", err)
	}
	if q.stmtRemove, err = db.Prepare(sqlRemove); err != nil {
		q.stmtEnqueue.Close()
		q.stmtDequeue.Close()
		q.stmtReEnqueue.Close()
		return nil, fmt.Errorf("prepare remove: %w", err)
	}

	return q, nil
}

// Enqueue adds path to the queue with the given mtime. If the path already has
// a pending row the existing row's mtime is updated in place (dedup); its
// position is preserved so FIFO ordering is maintained. A path whose row is
// currently 'processing' gets a fresh pending row inserted alongside it.
func (q *Queue) Enqueue(path string, mtime int64) error {
	now := time.Now().UTC().UnixMilli()
	_, err := q.stmtEnqueue.Exec(path, mtime, now, now)
	if err != nil {
		return fmt.Errorf("enqueue %q: %w", path, err)
	}
	return nil
}

// Dequeue returns the oldest pending entry and marks it as 'processing'.
// Returns nil, nil when the queue has no pending entries.
func (q *Queue) Dequeue() (*Entry, error) {
	row := q.stmtDequeue.QueryRow()
	e := &Entry{}
	err := row.Scan(&e.ID, &e.FilePath, &e.Mtime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue: %w", err)
	}
	return e, nil
}

// ReEnqueue moves a processing row back to pending at the back of the queue.
// Returns an error if no row with the given ID exists.
func (q *Queue) ReEnqueue(id int64) error {
	res, err := q.stmtReEnqueue.Exec(id)
	if err != nil {
		return fmt.Errorf("re-enqueue id=%d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("re-enqueue rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("re-enqueue: no row with id=%d", id)
	}
	return nil
}

// Remove deletes the queue entry with the given ID after successful processing.
// Returns an error if no row with the given ID exists.
func (q *Queue) Remove(id int64) error {
	res, err := q.stmtRemove.Exec(id)
	if err != nil {
		return fmt.Errorf("remove id=%d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("remove: no row with id=%d", id)
	}
	return nil
}

// Close releases all prepared statements. It does not close the underlying DB.
func (q *Queue) Close() {
	q.stmtEnqueue.Close()
	q.stmtDequeue.Close()
	q.stmtReEnqueue.Close()
	q.stmtRemove.Close()
}
