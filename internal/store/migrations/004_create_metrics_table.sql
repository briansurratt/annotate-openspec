-- down:
DROP TABLE IF EXISTS metrics;
-- /down
CREATE TABLE IF NOT EXISTS metrics (
    id           INTEGER  PRIMARY KEY AUTOINCREMENT,
    metric_name  TEXT     NOT NULL,
    value        REAL     NOT NULL,
    timestamp    TEXT     NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
