-- down:
DROP INDEX IF EXISTS idx_metrics_name;
-- /down
CREATE INDEX IF NOT EXISTS idx_metrics_name ON metrics (metric_name);
