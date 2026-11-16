DELETE FROM jobs WHERE kind = 'voiceover.recording';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing', 'media.import', 'youtube.sync'));
DELETE FROM voiceovers WHERE status = 'uploading';
CREATE OR REPLACE FUNCTION queue_voiceover_audio_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.storage_key IS NOT NULL THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.storage_key);
    END IF;
    RETURN OLD;
END;
$$;
ALTER TABLE voiceovers DROP COLUMN recording_key;
ALTER TABLE voiceovers DROP CONSTRAINT voiceovers_status_check;
ALTER TABLE voiceovers ADD CONSTRAINT voiceovers_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed'));
