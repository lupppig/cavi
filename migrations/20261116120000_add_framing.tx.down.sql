DROP TABLE media_framing;
DELETE FROM jobs WHERE kind = 'media.framing';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak', 'plans.create'));
ALTER TABLE media_assets DROP COLUMN framing_error, DROP COLUMN framing_status;
