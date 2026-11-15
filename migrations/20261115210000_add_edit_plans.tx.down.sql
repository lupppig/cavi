DROP TABLE edit_plans;
DROP TABLE plan_runs;
DELETE FROM jobs WHERE kind = 'plans.create';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak'));
