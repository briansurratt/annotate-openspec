-- down:
DROP INDEX IF EXISTS idx_queue_file_path;
-- /down
CREATE INDEX IF NOT EXISTS idx_queue_file_path ON queue (file_path);
