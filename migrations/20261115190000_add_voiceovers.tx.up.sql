-- Voiceovers: a script spoken by an AI voice, stored as audio for montages.
-- Jobs gain a generic subject_id for work on resources other than media
-- files and renders; handlers re-check the subject belongs to the job's
-- organisation, so it carries no foreign key.

ALTER TABLE jobs ADD COLUMN subject_id uuid;
CREATE INDEX jobs_subject_id_idx ON jobs (subject_id) WHERE subject_id IS NOT NULL;
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak'));

CREATE TABLE voiceovers (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id      uuid        NOT NULL,
    project_id           uuid        NOT NULL,
    script               text        NOT NULL,
    voice                text        NOT NULL,
    status               text        NOT NULL DEFAULT 'queued',
    provider             text        NOT NULL,
    model                text        NOT NULL,
    prompt_version       text        NOT NULL,
    storage_key          text,
    duration_ms          bigint,
    size_bytes           bigint,
    error                text,
    requested_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz,

    CONSTRAINT voiceovers_id_organisation_key UNIQUE (id, organisation_id),
    CONSTRAINT voiceovers_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    CONSTRAINT voiceovers_script_length CHECK (char_length(script) BETWEEN 1 AND 1200),
    CONSTRAINT voiceovers_ready_check CHECK (status <> 'ready' OR (storage_key IS NOT NULL AND duration_ms > 0)),
    CONSTRAINT voiceovers_project_fkey FOREIGN KEY (project_id, organisation_id)
        REFERENCES projects (id, organisation_id) ON DELETE CASCADE
);
CREATE INDEX voiceovers_project_created_idx ON voiceovers (project_id, created_at DESC);

CREATE FUNCTION queue_voiceover_audio_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.storage_key IS NOT NULL THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.storage_key);
    END IF;
    RETURN OLD;
END;
$$;

CREATE TRIGGER voiceovers_queue_audio_deletion
    AFTER DELETE ON voiceovers
    FOR EACH ROW EXECUTE FUNCTION queue_voiceover_audio_deletion();
