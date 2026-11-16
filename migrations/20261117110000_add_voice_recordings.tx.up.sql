-- Recordings: a voiceover can be the creator's own voice, recorded in the
-- browser and uploaded straight to storage (recording_key) while
-- 'uploading', then decoded, trimmed and timed by the media worker
-- (voiceover.recording) into the usual WAV (storage_key).
ALTER TABLE voiceovers DROP CONSTRAINT voiceovers_status_check;
ALTER TABLE voiceovers ADD CONSTRAINT voiceovers_status_check CHECK (status IN ('uploading', 'queued', 'running', 'ready', 'failed'));
ALTER TABLE voiceovers ADD COLUMN recording_key text;

CREATE OR REPLACE FUNCTION queue_voiceover_audio_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.storage_key IS NOT NULL THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.storage_key);
    END IF;
    IF OLD.recording_key IS NOT NULL THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.recording_key);
    END IF;
    RETURN OLD;
END;
$$;

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing', 'media.import', 'youtube.sync', 'voiceover.recording'));
