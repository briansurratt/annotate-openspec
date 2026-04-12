-- down:
DROP TABLE IF EXISTS index_cache;
-- /down
CREATE TABLE IF NOT EXISTS index_cache (
    file_path  TEXT  PRIMARY KEY,
    mtime      TEXT  NOT NULL,
    hash       TEXT  NOT NULL,
    data       BLOB  NOT NULL DEFAULT ''
);
