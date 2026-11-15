-- Edit plans: AI-proposed montages for a project, from its transcripts and
-- creator brief. One current run per project; requesting again replaces
-- it. The brief and the files planned from are copied onto the run.

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak', 'plans.create'));

CREATE TABLE plan_runs (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id      uuid        NOT NULL,
    project_id           uuid        NOT NULL,
    status               text        NOT NULL DEFAULT 'queued',
    provider             text        NOT NULL,
    model                text        NOT NULL,
    prompt_version       text        NOT NULL,
    objective            text        NOT NULL DEFAULT '',
    min_ms               bigint      NOT NULL,
    max_ms               bigint      NOT NULL,
    media_asset_ids      uuid[]      NOT NULL,
    campaign             jsonb,
    error                text,
    requested_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz,

    CONSTRAINT plan_runs_project_key UNIQUE (project_id),
    CONSTRAINT plan_runs_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    CONSTRAINT plan_runs_length_check CHECK (min_ms > 0 AND max_ms > min_ms),
    CONSTRAINT plan_runs_files_check CHECK (cardinality(media_asset_ids) BETWEEN 1 AND 5),
    CONSTRAINT plan_runs_project_fkey FOREIGN KEY (project_id, organisation_id)
        REFERENCES projects (id, organisation_id) ON DELETE CASCADE
);

-- Plans, best first. segments holds [{source_asset_id, start_ms, end_ms,
-- first_segment, last_segment}], copied from the transcripts.
CREATE TABLE edit_plans (
    plan_run_id      uuid    NOT NULL REFERENCES plan_runs (id) ON DELETE CASCADE,
    position         integer NOT NULL,
    title            text    NOT NULL,
    hook             text    NOT NULL,
    voiceover_script text    NOT NULL,
    reason           text    NOT NULL,
    campaign_link    text    NOT NULL,
    segments         jsonb   NOT NULL,
    PRIMARY KEY (plan_run_id, position)
);
