-- Transcription: the first AI task.

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('media.process', 'media.transcribe'));

-- Which supported model an organisation uses for each AI task. No row means
-- the platform default. Values are validated against the code's catalogue
-- when written, and again when resolved.
CREATE TABLE organisation_ai_models (
    organisation_id    uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    task               text        NOT NULL,
    provider           text        NOT NULL,
    model              text        NOT NULL,
    updated_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, task)
);

-- One current transcript per media file. The provider, model and prompt
-- version are resolved when transcription is requested and kept, so
-- retries use the same model and results are traceable.
CREATE TABLE transcripts (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id      uuid        NOT NULL,
    media_asset_id       uuid        NOT NULL,
    status               text        NOT NULL DEFAULT 'queued',
    provider             text        NOT NULL,
    model                text        NOT NULL,
    prompt_version       text        NOT NULL,
    language             text,
    error                text,
    requested_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz,

    CONSTRAINT transcripts_media_asset_key UNIQUE (media_asset_id),
    CONSTRAINT transcripts_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    CONSTRAINT transcripts_media_asset_fkey FOREIGN KEY (media_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE CASCADE
);

CREATE TABLE transcript_segments (
    transcript_id uuid    NOT NULL REFERENCES transcripts (id) ON DELETE CASCADE,
    position      integer NOT NULL,
    start_ms      bigint  NOT NULL,
    end_ms        bigint  NOT NULL,
    speaker       text    NOT NULL,
    text          text    NOT NULL,
    PRIMARY KEY (transcript_id, position),
    CONSTRAINT transcript_segments_times_check CHECK (start_ms >= 0 AND end_ms > start_ms)
);

-- AI usage for cost accounting, attributed to the organisation that pays.
CREATE TABLE ai_usage (
    id              bigserial   PRIMARY KEY,
    organisation_id uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    job_id          uuid,
    task            text        NOT NULL,
    provider        text        NOT NULL,
    model           text        NOT NULL,
    input_tokens    bigint      NOT NULL DEFAULT 0,
    output_tokens   bigint      NOT NULL DEFAULT 0,
    audio_ms        bigint      NOT NULL DEFAULT 0,
    succeeded       boolean     NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_usage_organisation_created_idx ON ai_usage (organisation_id, created_at DESC);
