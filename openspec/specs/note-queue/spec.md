### Requirement: Queue Construction
The system SHALL construct a `Queue` by accepting a `*sql.DB` and preparing all statements at creation time.

#### Scenario: Successful construction
- **WHEN** `queue.New(db)` is called with an initialized `*sql.DB`
- **THEN** a `*Queue` is returned with all four prepared statements ready
- **AND** no error is returned

#### Scenario: Construction fails on bad DB
- **WHEN** `queue.New(db)` is called with a nil or closed `*sql.DB`
- **THEN** an error is returned
- **AND** no `*Queue` is returned

### Requirement: Enqueue New Path
The system SHALL insert a new row when enqueuing a path that is not already in the queue.

#### Scenario: Enqueue a new file path
- **WHEN** `Enqueue(path, mtime)` is called with a path not currently in the queue
- **THEN** a new row is inserted with `status = 'pending'`, the given `mtime`, and a monotonically increasing `position`
- **AND** no error is returned

### Requirement: Enqueue Deduplication
The system SHALL deduplicate by updating the existing row's `mtime` when a path is already pending in the queue, rather than inserting a duplicate.

#### Scenario: Enqueue a path already in the queue
- **WHEN** `Enqueue(path, mtime)` is called with a path that already has a `pending` row
- **THEN** the existing row's `mtime` is updated to the new value
- **AND** no new row is inserted
- **AND** the row's `position` is unchanged (preserving FIFO order)

#### Scenario: Enqueue a path currently being processed
- **WHEN** `Enqueue(path, mtime)` is called with a path whose row has `status = 'processing'`
- **THEN** a new `pending` row is inserted for that path
- **AND** the `processing` row is left unchanged

### Requirement: Dequeue Front Entry
The system SHALL dequeue the oldest pending entry by returning its data and marking it as processing.

#### Scenario: Dequeue from non-empty queue
- **WHEN** `Dequeue()` is called and at least one `pending` row exists
- **THEN** the row with the lowest `position` and `status = 'pending'` is selected
- **AND** that row's `status` is updated to `'processing'`
- **AND** the row's `id`, `file_path`, and `mtime` are returned

#### Scenario: Dequeue from empty queue
- **WHEN** `Dequeue()` is called and no `pending` rows exist
- **THEN** `nil, nil` is returned (no entry, no error)

### Requirement: Re-enqueue on Conflict
The system SHALL move a processing entry back to pending status when a conflict is detected, appending it to the back of the queue.

#### Scenario: Re-enqueue a processing entry
- **WHEN** `ReEnqueue(id)` is called with the `id` of a `processing` row
- **THEN** that row's `status` is set back to `'pending'`
- **AND** its `position` is updated to be greater than all current positions (appended to back)

#### Scenario: Re-enqueue a non-existent entry
- **WHEN** `ReEnqueue(id)` is called with an `id` that does not exist
- **THEN** an error is returned

### Requirement: Remove on Success
The system SHALL delete a queue entry after successful processing.

#### Scenario: Remove a completed entry
- **WHEN** `Remove(id)` is called with the `id` of an existing row
- **THEN** the row is deleted from the queue table
- **AND** no error is returned

#### Scenario: Remove a non-existent entry
- **WHEN** `Remove(id)` is called with an `id` that does not exist
- **THEN** an error is returned
