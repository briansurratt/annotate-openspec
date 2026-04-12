-- down:
DROP TABLE IF EXISTS event_log;
-- /down
CREATE TABLE IF NOT EXISTS event_log (
    id          INTEGER  PRIMARY KEY AUTOINCREMENT,
    event_type  TEXT     NOT NULL,
    file_path   TEXT     NOT NULL,
    details     TEXT     NOT NULL DEFAULT '',
    timestamp   TEXT     NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
