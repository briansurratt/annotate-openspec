-- down:
DROP INDEX IF EXISTS idx_queue_file_path_pending;
CREATE INDEX IF NOT EXISTS idx_queue_file_path ON queue (file_path);
-- /down
DROP INDEX IF EXISTS idx_queue_file_path;
CREATE UNIQUE INDEX IF NOT EXISTS idx_queue_file_path_pending ON queue (file_path) WHERE status = 'pending';
