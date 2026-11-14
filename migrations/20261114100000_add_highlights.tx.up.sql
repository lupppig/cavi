-- Highlight detection: candidate clips found in a file's transcript.

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights'));

-- One current highlight run per media file; requesting again replaces it.
-- The brief (objective and clip length) is kept with the provider, model
-- and prompt version, so results can be traced to what was asked.
CREATE TABLE highlight_runs (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id      uuid        NOT NULL,
    media_asset_id       uuid        NOT NULL,
    status               text        NOT NULL DEFAULT 'queued',
    provider             text        NOT NULL,
    model                text        NOT NULL,
    prompt_version       text        NOT NULL,
    objective            text        NOT NULL DEFAULT '',
    min_ms               bigint      NOT NULL,
    max_ms               bigint      NOT NULL,
    error                text,
    requested_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz,

    CONSTRAINT highlight_runs_media_asset_key UNIQUE (media_asset_id),
    CONSTRAINT highlight_runs_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    CONSTRAINT highlight_runs_length_check CHECK (min_ms > 0 AND max_ms > min_ms),
    CONSTRAINT highlight_runs_media_asset_fkey FOREIGN KEY (media_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE CASCADE
);

-- Candidate clips, best first. Times are copied from the transcript lines
-- the clip spans, so they stay valid if the file is transcribed again.
CREATE TABLE highlights (
    highlight_run_id uuid    NOT NULL REFERENCES highlight_runs (id) ON DELETE CASCADE,
    position         integer NOT NULL,
    start_ms         bigint  NOT NULL,
    end_ms           bigint  NOT NULL,
    first_segment    integer NOT NULL,
    last_segment     integer NOT NULL,
    title            text    NOT NULL,
    reason           text    NOT NULL,
    PRIMARY KEY (highlight_run_id, position),
    CONSTRAINT highlights_times_check CHECK (start_ms >= 0 AND end_ms > start_ms),
    CONSTRAINT highlights_segments_check CHECK (first_segment >= 0 AND last_segment >= first_segment)
);
