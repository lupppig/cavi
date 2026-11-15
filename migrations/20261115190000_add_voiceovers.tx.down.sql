INSERT INTO storage_deletions (storage_key) SELECT storage_key FROM voiceovers WHERE storage_key IS NOT NULL;
DROP TRIGGER voiceovers_queue_audio_deletion ON voiceovers;
DROP TABLE voiceovers;
DROP FUNCTION queue_voiceover_audio_deletion();
DELETE FROM jobs WHERE kind = 'voiceover.speak';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read'));
DROP INDEX jobs_subject_id_idx;
ALTER TABLE jobs DROP COLUMN subject_id;
