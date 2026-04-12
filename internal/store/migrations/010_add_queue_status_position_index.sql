-- down:
DROP INDEX IF EXISTS idx_queue_status_position;
-- /down
CREATE INDEX IF NOT EXISTS idx_queue_status_position ON queue (status, position);
