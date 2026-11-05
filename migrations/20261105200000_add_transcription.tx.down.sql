DROP TABLE ai_usage;
DROP TABLE transcript_segments;
DROP TABLE transcripts;
DROP TABLE organisation_ai_models;
DELETE FROM jobs WHERE kind = 'media.transcribe';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('media.process'));
