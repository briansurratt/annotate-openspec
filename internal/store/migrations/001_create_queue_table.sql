-- down:
DROP TABLE IF EXISTS queue;
-- /down
CREATE TABLE IF NOT EXISTS queue (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    file_path  TEXT     NOT NULL,
    mtime      TEXT     NOT NULL,
    status     TEXT     NOT NULL DEFAULT 'pending',
    created_at TEXT     NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT     NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
