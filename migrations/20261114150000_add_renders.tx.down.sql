DELETE FROM jobs WHERE kind = 'render.clip';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights'));
ALTER TABLE jobs DROP COLUMN render_id;
-- Keep rendered outputs from being orphaned in storage.
INSERT INTO storage_deletions (storage_key) SELECT output_key FROM renders WHERE output_key IS NOT NULL;
DROP TRIGGER renders_queue_output_deletion ON renders;
DROP TABLE renders;
DROP FUNCTION queue_render_output_deletion();
