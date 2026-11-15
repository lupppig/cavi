DELETE FROM jobs WHERE kind = 'brief.read';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip'));
DROP TABLE brief_rules;
DROP TABLE briefs;
