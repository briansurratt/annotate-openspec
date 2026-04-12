-- down:
ALTER TABLE queue DROP COLUMN position;
-- /down
ALTER TABLE queue ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
