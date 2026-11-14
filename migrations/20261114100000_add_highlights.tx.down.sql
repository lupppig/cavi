DELETE FROM organisation_ai_models WHERE task = 'highlights';
DROP TABLE highlights;
DROP TABLE highlight_runs;
DELETE FROM jobs WHERE kind = 'media.highlights';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('media.process', 'media.transcribe'));
