-- down:
DROP INDEX IF EXISTS idx_event_log_timestamp;
-- /down
CREATE INDEX IF NOT EXISTS idx_event_log_timestamp ON event_log (timestamp);
